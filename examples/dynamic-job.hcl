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
  default = "http://host.docker.internal:4646"
}

variable "docker_extra_hosts" {
  type    = list(string)
  default = ["host.docker.internal:host-gateway"]
}

job "dynamic" {
  name        = "dynamic-job"
  datacenters = var.datacenters
  type        = "batch"

  group "▶️" {
    task "init" {
      driver = "docker"

      config {
        image = var.image
        args  = ["agent", "init"]

        extra_hosts    = var.docker_extra_hosts
        auth_soft_fail = true
      }

      env {
        NOMAD_ADDR           = var.nomad_addr
        NOMAD_PIPELINE_DEBUG = "true"
      }
    }
  }

  group "1-generate-tasks" {
    count = 0

    meta = {
      "nomad-pipeline/root"          = "true"
      "nomad-pipeline/dynamic-tasks" = "tasks/*"
    }

    task "generate-tasks" {
      driver = "raw_exec"

      config {
        command  = "/bin/bash"
        args     = ["${NOMAD_TASK_DIR}/generate-tasks.sh"]
      }

      template {
        data = <<-EOT
        [
          {
            "Name": "${TASK_NAME}",
            "Count": 0,
            "Meta": {
              "nomad-pipeline/root": "true",
              "nomad-pipeline/next": "3-last"
            },
            "Tasks": [
              {
                "Name": "echo",
                "Driver": "raw_exec",
                "Config": {
                  "command": "/bin/echo",
                  "args": [ "hey" ]
                }
              }
            ]
          }
        ]
        EOT

        destination = "${NOMAD_TASK_DIR}/task.template.json"
      }

      template {
        data = <<-EOT
        [
          {
            "Name": "3-last",
            "Count": 0,
            "Meta": {
              "nomad-pipeline/root": "true",
              "nomad-pipeline/dependencies": "2a-echo,2b-echo"
            },
            "Tasks": [
              {
                "Name": "echo",
                "Driver": "raw_exec",
                "Config": {
                  "command": "/bin/echo",
                  "args": [ "hey" ]
                }
              }
            ]
          }
        ]
        EOT

        destination = "${NOMAD_TASK_DIR}/last-task.template.json"
      }

      template {
        data = <<-EOT
        #!/bin/bash
        mkdir -p "${NOMAD_ALLOC_DIR}/tasks"
        cat ${NOMAD_TASK_DIR}/task.template.json | TASK_NAME="2a-echo" envsubst > ${NOMAD_ALLOC_DIR}/tasks/2a-echo.json
        cat ${NOMAD_TASK_DIR}/task.template.json | TASK_NAME="2b-echo" envsubst > ${NOMAD_ALLOC_DIR}/tasks/2b-echo.json
        cp ${NOMAD_TASK_DIR}/last-task.template.json ${NOMAD_ALLOC_DIR}/tasks/3-last.json
        sleep 10
        EOT

        destination = "${NOMAD_TASK_DIR}/generate-tasks.sh"
      }
    }
  }
}