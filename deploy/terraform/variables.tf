variable "tenancy_ocid" {
  description = "OCI tenancy OCID"
  type        = string
  sensitive   = true
}

variable "user_ocid" {
  description = "OCI user OCID"
  type        = string
  sensitive   = true
}

variable "fingerprint" {
  description = "OCI API fingerprint"
  type        = string
  sensitive   = true
}

variable "private_key_path" {
  description = "Path to OCI API private key"
  type        = string
  sensitive   = true
}

variable "region" {
  description = "OCI region"
  type        = string
  default     = "us-ashburn-1"
}

variable "existing_subnet_id" {
  description = "Existing OCI subnet OCID to deploy into. Leave empty to create ADM networking."
  type        = string
  default     = ""
}

variable "reuse_discovered_network" {
  description = "When existing_subnet_id is empty, reuse an adm-vcn/adm-subnet already present in the tenancy instead of creating a new VCN. Must be disabled when this state already manages the network, or Terraform would plan to destroy it."
  type        = bool
  default     = true
}

variable "availability_domain_index" {
  description = "Which availability domain to launch in (0-based). Cycle this on a re-dispatch to work around 'Out of host capacity' in a specific AD. Clamped to the ADs that exist; single-AD regions like ap-tokyo-1 ignore it."
  type        = number
  default     = 0
}

variable "force_shape" {
  description = "Set to \"micro\" to force VM.Standard.E2.1.Micro instead of A1.Flex, e.g. when the region is out of A1 host capacity. Empty picks automatically."
  type        = string
  default     = ""

  validation {
    condition     = contains(["", "micro"], var.force_shape)
    error_message = "force_shape must be \"\" or \"micro\"."
  }
}

variable "ocpus" {
  description = "Number of OCPUs (max 4 for Always Free)"
  type        = number
  default     = 4

  validation {
    condition     = var.ocpus >= 1 && var.ocpus <= 4
    error_message = "Always Free tier allows 1-4 OCPUs."
  }
}

variable "memory_in_gbs" {
  description = "Memory in GB (max 24 for Always Free)"
  type        = number
  default     = 24

  validation {
    condition     = var.memory_in_gbs >= 1 && var.memory_in_gbs <= 24
    error_message = "Always Free tier allows 1-24 GB memory."
  }
}

variable "volume_size_gbs" {
  description = "Block volume size in GB (max 200 for Always Free)"
  type        = number
  default     = 100

  validation {
    condition     = var.volume_size_gbs >= 50 && var.volume_size_gbs <= 200
    error_message = "Always Free tier allows up to 200 GB."
  }
}

variable "ssh_public_key" {
  description = "SSH public key for instance access"
  type        = string
}

variable "docker_compose_version" {
  description = "Docker Compose version to install"
  type        = string
  default     = "v2.29.1"
}
