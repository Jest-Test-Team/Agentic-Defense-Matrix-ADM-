output "instance_public_ip" {
  description = "Public IP of the ADM instance"
  value       = oci_core_instance.adm_instance.public_ip
}

output "instance_private_ip" {
  description = "Private IP of the ADM instance"
  value       = oci_core_instance.adm_instance.private_ip
}

output "instance_id" {
  description = "OCID of the ADM instance"
  value       = oci_core_instance.adm_instance.id
}

output "volume_id" {
  description = "OCID of the data volume"
  value       = oci_core_volume.adm_volume.id
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "ssh -i ~/.ssh/adm_key ubuntu@${oci_core_instance.adm_instance.public_ip}"
}

output "gateway_url" {
  description = "Gateway URL"
  value       = "http://${oci_core_instance.adm_instance.public_ip}:8080"
}

output "ollama_url" {
  description = "Ollama URL"
  value       = "http://${oci_core_instance.adm_instance.public_ip}:11434"
}

output "next_steps" {
  description = "Next steps after deployment"
  value       = <<-EOT
    1. SSH into the instance:
       ssh -i ~/.ssh/adm_key ubuntu@${oci_core_instance.adm_instance.public_ip}

    2. Check deployment status:
       sudo -u adm /opt/adm/scripts/status.sh

    3. View logs:
       sudo -u adm /opt/adm/scripts/logs.sh

    4. Access the Gateway:
       http://${oci_core_instance.adm_instance.public_ip}:8080/v1/health
  EOT
}
