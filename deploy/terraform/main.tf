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

resource "oci_core_vcn" "adm_vcn" {
  compartment_id = var.tenancy_ocid
  cidr_block     = "10.0.0.0/16"
  display_name   = "adm-vcn"
}

resource "oci_core_internet_gateway" "adm_igw" {
  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn.id
  display_name   = "adm-igw"
  enabled        = true
}

resource "oci_core_route_table" "adm_rt" {
  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn.id
  display_name   = "adm-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    network_entity_id = oci_core_internet_gateway.adm_igw.id
  }
}

resource "oci_core_security_list" "adm_sl" {
  compartment_id = var.tenancy_ocid
  vcn_id         = oci_core_vcn.adm_vcn.id
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
  compartment_id      = var.tenancy_ocid
  vcn_id              = oci_core_vcn.adm_vcn.id
  cidr_block          = "10.0.0.0/24"
  display_name        = "adm-subnet"
  route_table_id      = oci_core_route_table.adm_rt.id
  security_list_ids   = [oci_core_security_list.adm_sl.id]
  dns_label           = "adm"
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
    subnet_id        = oci_core_subnet.adm_subnet.id
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
    is_monitoring_disabled  = false
  }
}

resource "oci_core_volume" "adm_volume" {
  compartment_id      = var.tenancy_ocid
  availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
  display_name        = "adm-data"
  size_in_gbs         = var.volume_size_gbs
  volume_backup_policy = "disabled"
}

resource "oci_core_volume_attachment" "adm_volume_attachment" {
  attachment_type = "paravirtualized"
  instance_id     = oci_core_instance.adm_instance.id
  volume_id       = oci_core_volume.adm_volume.id
}
