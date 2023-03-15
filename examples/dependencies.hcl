variable "datacenters" {
  type    = list(string)
  default = ["dc1"]
}

variable "image" {
  type    = string
  default = "ghcr.io/hyperbadger/nomad-pipeline:main"
}

variable "nomad_addr" {
  type    = string
  default = "http://localhost:4646"
}

job "dependencies" {
  name        = "dependencies"
  datacenters = var.datacenters
  type        = "batch"

  meta = {
    "nomad-pipeline.enabled" = "true"
  }

  group "▶️" {
    task "init" {
      driver = "docker"

      config {
        image = var.image
        args  = ["agent", "init"]

        network_mode = var.nomad_addr == "http://localhost:4646" ? "host" : null
      }

      env {
        NOMAD_ADDR           = var.nomad_addr
        NOMAD_PIPELINE_DEBUG = "true"
      }
    }
  }

  group "1a-task" {
    count = 0

    meta = {
      "nomad-pipeline.root" = "true"
      "nomad-pipeline.next" = "2-dependent"
    }

    task "normal" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "do something"
        sleep 5

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "1b-task" {
    count = 0

    meta = {
      "nomad-pipeline.root" = "true"
      "nomad-pipeline.next" = "2-dependent"
    }

    task "normal" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "do something"
        sleep 10

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "1c-task" {
    count = 0

    meta = {
      "nomad-pipeline.root" = "true"
      "nomad-pipeline.next" = "2-dependent"
    }

    task "normal" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash

        echo "do something"
        sleep 60

        EOT

        destination = "local/main.sh"
      }
    }
  }

  group "2-dependent" {
    count = 0

    meta = {
      "nomad-pipeline.dependencies" = "1a-task, 1b-task, 1c-task"
    }

    task "dependent" {
      driver = "raw_exec"

      config {
        command = "/bin/bash"
        args = ["local/main.sh"]
      }

      template {
        data = <<-EOT
        #!/bin/bash
        echo "successfully waited for dependency"
        EOT

        destination = "local/main.sh"
      }
    }
  }
}
