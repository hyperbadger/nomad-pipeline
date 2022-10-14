variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
}

variable "nomad_addr" {
  type    = string
  default = "http://host.docker.internal:4646"
}

variable "docker_extra_hosts" {
  type    = list(string)
  default = ["host.docker.internal:host-gateway"]
}

job "triggered" {
  name        = "triggered-job"
  datacenters = var.datacenters
  type        = "batch"

  meta = {
    "nomad-pipeline.enabled" = "true"
  }

  parameterized {
    payload       = "forbidden"
    meta_required = [
      "name",
      "object_path",
    ]
  }

  group "basic-task" {
    count = 1

    task "normal" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "name -> ${NOMAD_META_name}"
        echo "object_path -> ${NOMAD_META_object_path}"

        EOT

        destination = "local/main.sh"
      }
    }
  }
}
