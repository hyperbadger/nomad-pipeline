variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
}

job "triggered-job" {
  name        = "triggered-job"
  datacenters = var.datacenters
  type        = "batch"

  meta = {
    "nomad-pipeline.enabled" = "true"
  }

  parameterized {
    payload       = "forbidden"
    meta_optional = ["name"]
    meta_required = ["object_path"]
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
