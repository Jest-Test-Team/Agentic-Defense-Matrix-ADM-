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
