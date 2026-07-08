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

data "oci_core_images" "ol8_image" {
  compartment_id           = var.tenancy_ocid
  operating_system         = "Oracle Linux"
  operating_system_version = "8"
  shape                    = "VM.Standard.A1.Flex"
}

# State is cached between CI runs and can be lost; a lost state would leave an
# orphaned adm-vcn that still counts against the tenancy vcn-count limit. Look
# for an existing adm-vcn/adm-subnet and reuse it instead of creating another.
data "oci_core_vcns" "existing_adm" {
  count = var.existing_subnet_id == "" && var.reuse_discovered_network ? 1 : 0

  compartment_id = var.tenancy_ocid
  display_name   = "adm-vcn"
  state          = "AVAILABLE"
}

data "oci_core_subnets" "existing_adm" {
  count = local.discovered_vcn_id != null ? 1 : 0

  compartment_id = var.tenancy_ocid
  vcn_id         = local.discovered_vcn_id
  display_name   = "adm-subnet"
  state          = "AVAILABLE"
}

# Diagnostics surfaced as plan outputs: actual vcn-count quota usage and every
# VCN/subnet in the compartment, so a LimitExceeded failure can be traced to
# what is consuming the quota without console access.
data "oci_limits_resource_availability" "vcn_count" {
  compartment_id = var.tenancy_ocid
  service_name   = "vcn"
  limit_name     = "vcn-count"
}

data "oci_core_vcns" "diagnostics" {
  compartment_id = var.tenancy_ocid
}

data "oci_core_subnets" "diagnostics" {
  for_each = { for v in data.oci_core_vcns.diagnostics.virtual_networks : v.id => v }

  compartment_id = var.tenancy_ocid
  vcn_id         = each.key
}

locals {
  discovered_vcn_id    = try(data.oci_core_vcns.existing_adm[0].virtual_networks[0].id, null)
  discovered_subnet_id = try(data.oci_core_subnets.existing_adm[0].subnets[0].id, null)

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

resource "oci_core_instance" "adm_instance" {
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
  compartment_id      = var.tenancy_ocid
  display_name        = "adm-instance"
  shape               = "VM.Standard.A1.Flex"

  shape_config {
    ocpus         = var.ocpus
    memory_in_gbs = var.memory_in_gbs
  }

  create_vnic_details {
    subnet_id        = local.subnet_id
    display_name     = "adm-vnic"
    assign_public_ip = true
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
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
  display_name        = "adm-data"
  size_in_gbs         = var.volume_size_gbs
}

resource "oci_core_volume_attachment" "adm_volume_attachment" {
  attachment_type = "paravirtualized"
  instance_id     = oci_core_instance.adm_instance.id
  volume_id       = oci_core_volume.adm_volume.id
}
