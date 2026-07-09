terraform {
  required_version = ">= 1.0"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = "~> 6.0"
    }
  }

  backend "local" {
    path = "terraform.tfstate"
  }
}

provider "oci" {
  region           = var.region
  tenancy_ocid     = var.tenancy_ocid
  user_ocid        = var.user_ocid
  fingerprint      = var.fingerprint
  private_key_path = var.private_key_path
}

data "oci_identity_availability_domains" "ads" {
  compartment_id = var.tenancy_ocid
}

locals {
  # A1 host capacity can be exhausted in one availability domain but free in
  # another. availability_domain_index lets a re-dispatch target a different AD
  # without editing code; it is clamped to the ADs that actually exist (regions
  # like ap-tokyo-1 have only one, in which case this is a no-op).
  ad_count            = length(data.oci_identity_availability_domains.ads.availability_domains)
  ad_index            = var.availability_domain_index % local.ad_count
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[local.ad_index].name
}

# The Always Free A1 allowance varies by tenancy and can be zero. Size the
# instance to what the compute limits actually allow, falling back to the
# free x86 micro shape when no A1 capacity is granted at all.
data "oci_limits_resource_availability" "a1_cores" {
  compartment_id = var.tenancy_ocid
  service_name   = "compute"
  limit_name     = "standard-a1-core-regional-count"
}

data "oci_limits_resource_availability" "a1_memory" {
  compartment_id = var.tenancy_ocid
  service_name   = "compute"
  limit_name     = "standard-a1-memory-regional-count"
}

locals {
  a1_cores_available  = try(tonumber(data.oci_limits_resource_availability.a1_cores.available), 0)
  a1_memory_available = try(tonumber(data.oci_limits_resource_availability.a1_memory.available), 0)

  instance_ocpus  = min(var.ocpus, local.a1_cores_available, local.a1_memory_available)
  instance_memory = min(var.memory_in_gbs, local.a1_memory_available)
  use_a1          = var.force_shape != "micro" && local.instance_ocpus >= 1

  instance_shape = local.use_a1 ? "VM.Standard.A1.Flex" : "VM.Standard.E2.1.Micro"
}

data "oci_core_images" "ol8_image" {
  compartment_id           = var.tenancy_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "8"
  shape                    = local.instance_shape
}

# The vcn-count limit is tenancy-wide but VCN listings are per-compartment, so
# search every accessible compartment for an existing adm-vcn/adm-subnet to
# reuse (state is cached between CI runs and can be lost, orphaning the VCN).
data "oci_identity_compartments" "all" {
  compartment_id            = var.tenancy_ocid
  compartment_id_in_subtree = true
  access_level              = "ACCESSIBLE"
  state                     = "ACTIVE"
}

locals {
  # Keyed by OCID suffix so full compartment OCIDs stay out of public CI logs.
  compartments = merge(
    { (substr(nonsensitive(var.tenancy_ocid), -12, 12)) = { id = nonsensitive(var.tenancy_ocid), name = "(root)" } },
    { for c in data.oci_identity_compartments.all.compartments : substr(c.id, -12, 12) => { id = c.id, name = c.name } }
  )
}

data "oci_core_vcns" "by_compartment" {
  for_each = local.compartments

  compartment_id = each.value.id
}

locals {
  all_vcns = flatten([
    for key, d in data.oci_core_vcns.by_compartment : [
      for v in d.virtual_networks : {
        id          = v.id
        compartment = local.compartments[key].name
        comp_id     = local.compartments[key].id
        name        = v.display_name
        state       = v.state
        cidr_blocks = v.cidr_blocks
      }
    ]
  ])
  vcns_by_id = { for v in local.all_vcns : v.id => v }
}

data "oci_core_subnets" "by_vcn" {
  for_each = local.vcns_by_id

  compartment_id = each.value.comp_id
  vcn_id         = each.key
}

# Custom quota policies override service limits and produce QuotaExceeded
# errors that cite the policy name (e.g. "bootVolumeQuota"); surface every
# policy and its statements to explain such failures.
data "oci_limits_quotas" "all" {
  compartment_id = var.tenancy_ocid
}

data "oci_limits_quota" "detail" {
  for_each = { for q in data.oci_limits_quotas.all.quotas : q.name => q.id }

  quota_id = each.value
}

# Storage diagnostics: the Always Free block storage allowance is shared by
# boot volumes, block volumes, and their backups across all compartments.
data "oci_limits_resource_availability" "block_storage_gb" {
  compartment_id      = var.tenancy_ocid
  service_name        = "block-storage"
  limit_name          = "total-storage-gb"
  availability_domain = local.availability_domain
}

data "oci_core_boot_volumes" "by_compartment" {
  for_each = local.compartments

  availability_domain = local.availability_domain
  compartment_id      = each.value.id
}

data "oci_core_volumes" "by_compartment" {
  for_each = local.compartments

  compartment_id = each.value.id
}

# Diagnostics surfaced as plan outputs: actual vcn-count quota usage and every
# VCN/subnet in the tenancy, so a LimitExceeded failure can be traced to what
# is consuming the quota without console access.
data "oci_limits_resource_availability" "vcn_count" {
  compartment_id = var.tenancy_ocid
  service_name   = "vcn"
  limit_name     = "vcn-count"
}

locals {
  reuse_network = var.existing_subnet_id == "" && var.reuse_discovered_network

  discovered_vcn_id = (
    local.reuse_network ?
    try([for v in local.all_vcns : v.id if v.name == "adm-vcn" && v.state == "AVAILABLE"][0], null) :
    null
  )
  adm_subnet_id = local.discovered_vcn_id == null ? null : try(
    [for s in data.oci_core_subnets.by_vcn[local.discovered_vcn_id].subnets : s.id if s.display_name == "adm-subnet"][0],
    null
  )

  # When the vcn-count quota is exhausted and no adm network exists, the only
  # way to deploy is borrowing a public subnet from an existing VCN. Required
  # port access is guaranteed by adm-nsg on the instance VNIC, so the borrowed
  # subnet's own security lists don't matter.
  public_subnets = flatten([
    for v in local.all_vcns : [
      for s in data.oci_core_subnets.by_vcn[v.id].subnets : {
        id   = s.id
        name = s.display_name
      } if v.state == "AVAILABLE" && !s.prohibit_public_ip_on_vnic
    ]
  ])
  fallback_subnet_id = (
    local.reuse_network && try(tonumber(data.oci_limits_resource_availability.vcn_count.available) < 1, false) ?
    try(
      [for s in local.public_subnets : s.id if length(regexall("public", lower(s.name))) > 0][0],
      local.public_subnets[0].id,
      null
    ) : null
  )

  discovered_subnet_id = local.adm_subnet_id != null ? local.adm_subnet_id : local.fallback_subnet_id

  create_network = var.existing_subnet_id == "" && local.discovered_subnet_id == null
  subnet_id = (
    var.existing_subnet_id != "" ? var.existing_subnet_id :
    local.discovered_subnet_id != null ? local.discovered_subnet_id :
    oci_core_subnet.adm_subnet[0].id
  )
}

resource "oci_core_vcn" "adm_vcn" {
  count = local.create_network ? 1 : 0

  compartment_id = var.tenancy_ocid
  cidr_block     = "10.0.0.0/16"
  display_name   = "adm-vcn"
  # Required for the subnet's dns_label: a subnet can only enable DNS when its
  # VCN was created with a dns_label.
  dns_label = "adm"

  lifecycle {
    precondition {
      condition     = try(tonumber(data.oci_limits_resource_availability.vcn_count.available) > 0, true)
      error_message = "The tenancy vcn-count quota is exhausted, so creating adm-vcn would fail. See the network_diagnostics output for what is using the quota, then reuse an existing subnet via existing_subnet_id (repo variable ADM_EXISTING_SUBNET_ID), delete an unused VCN, or request a limit increase."
    }
  }
}

resource "oci_core_internet_gateway" "adm_igw" {
  count = local.create_network ? 1 : 0

  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn[0].id
  display_name   = "adm-igw"
  enabled        = true
}

resource "oci_core_route_table" "adm_rt" {
  count = local.create_network ? 1 : 0

  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn[0].id
  display_name   = "adm-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    network_entity_id = oci_core_internet_gateway.adm_igw[0].id
  }
}

resource "oci_core_security_list" "adm_sl" {
  count = local.create_network ? 1 : 0

  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn[0].id
  display_name   = "adm-sl"

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 22
      max = 22
    }
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 8080
      max = 8080
    }
  }

  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 11434
      max = 11434
    }
  }

  egress_security_rules {
    protocol    = "all"
    destination = "0.0.0.0/0"
  }
}

resource "oci_core_subnet" "adm_subnet" {
  count = local.create_network ? 1 : 0

  compartment_id    = var.tenancy_ocid
  vcn_id            = oci_core_vcn.adm_vcn[0].id
  cidr_block        = "10.0.0.0/24"
  display_name      = "adm-subnet"
  route_table_id    = oci_core_route_table.adm_rt[0].id
  security_list_ids = [oci_core_security_list.adm_sl[0].id]
  dns_label         = "adm"
}

# The selected subnet may be borrowed from a foreign VCN whose security lists
# don't open ADM's ports. NSG rules are unioned with security lists and apply
# only to VNICs attached to the NSG, so adm-nsg guarantees access for this
# instance without touching the shared subnet's rules.
data "oci_core_subnet" "selected" {
  subnet_id = local.subnet_id
}

resource "oci_core_network_security_group" "adm_nsg" {
  compartment_id = data.oci_core_subnet.selected.compartment_id
  vcn_id         = data.oci_core_subnet.selected.vcn_id
  display_name   = "adm-nsg"
}

resource "oci_core_network_security_group_security_rule" "adm_nsg_ingress" {
  for_each = { ssh = 22, gateway = 8080, ollama = 11434, analysis = 8090 }

  network_security_group_id = oci_core_network_security_group.adm_nsg.id
  direction                 = "INGRESS"
  protocol                  = "6"
  source                    = "0.0.0.0/0"
  source_type               = "CIDR_BLOCK"

  tcp_options {
    destination_port_range {
      min = each.value
      max = each.value
    }
  }
}

resource "oci_core_network_security_group_security_rule" "adm_nsg_egress" {
  network_security_group_id = oci_core_network_security_group.adm_nsg.id
  direction                 = "EGRESS"
  protocol                  = "all"
  destination               = "0.0.0.0/0"
  destination_type          = "CIDR_BLOCK"
}

resource "oci_core_instance" "adm_instance" {
  # The boot volume and adm_volume share the block storage quota; when a size
  # change replaces adm_volume, the old one must be gone before the instance
  # requests its boot volume or the combined usage can exceed the cap.
  depends_on = [oci_core_volume.adm_volume]

  availability_domain = local.availability_domain
  compartment_id      = var.tenancy_ocid
  display_name        = "adm-instance"
  shape               = local.instance_shape

  # Fixed shapes like E2.1.Micro reject shape_config; only Flex shapes take one.
  dynamic "shape_config" {
    for_each = local.use_a1 ? [1] : []

    content {
      ocpus         = local.instance_ocpus
      memory_in_gbs = local.instance_memory
    }
  }

  create_vnic_details {
    subnet_id        = local.subnet_id
    display_name     = "adm-vnic"
    assign_public_ip = true
    nsg_ids          = [oci_core_network_security_group.adm_nsg.id]
  }

  source_details {
    source_type = "image"
    source_id   = data.oci_core_images.ol8_image.images[0].id
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    user_data = base64encode(templatefile("${path.module}/cloud-init.yaml", {
      docker_compose_version = var.docker_compose_version
    }))
  }

  agent_config {
    are_all_plugins_disabled = false
    is_monitoring_disabled   = false
  }
}

resource "oci_core_volume" "adm_volume" {
  compartment_id      = var.tenancy_ocid
  availability_domain = local.availability_domain
  display_name        = "adm-data"
  size_in_gbs         = var.volume_size_gbs
}

resource "oci_core_volume_attachment" "adm_volume_attachment" {
  attachment_type = "paravirtualized"
  instance_id     = oci_core_instance.adm_instance.id
  volume_id       = oci_core_volume.adm_volume.id
}
