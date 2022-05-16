# Nomad Pipeline

Nomad is great for batch jobs, however in its current state, you can't have job dependecies which is required when running pipeline style workload. The inspiration for this project came from the [`nomad-dag-hack`](https://github.com/cgbaker/nomad-dag-hack) project and the accompanying [blog post](https://www.cgbaker.net/2020/12/hacking-nomad-job-dependencies/).

![](examples/happy-job.gif)

## How to get started?

It's just 2 steps...

**Inject the 'init' task group**

The 'init' task will look at all the meta-tags setup in the next step and inject lifecycle task hooks into the task groups. The hooks are responsible for starting the next task group after the current one finishes.

```hcl
group "▶️" {
  count = 1

  task "init" {
    driver = "docker"

    config {
      image = "ghcr.io/hyperbadger/nomad-pipeline:main"
      args  = ["-init"]
    }

    env {
      NOMAD_ADDR           = var.nomad_addr
    }
  }
}
```

**Annotate task groups with meta-tags**

```hcl
group "1-first-task-group" {
  count = 0  # <-- Important! nomad-pipeline will control the count

  meta = {
    "nomad-pipeline/root" = "true"  # <-- Indicates the starting task group
    "nomad-pipeline/next" = "2-second-task-group"
  }

  ...
}

group "2-second-task-group" {
  count = 0

  ...
}
```

## How to run examples?

**Requirements**

- Docker (with default `bridge` network)
- Nomad
- jq

**Steps**

1. Find your Docker `bridge` network gateway IP - `export DOCKER_GATEWAY_IP=$(docker network inspect bridge | jq -r ".[].IPAM.Config[].Gateway")`
1. Start Nomad in dev mode - `nomad agent -dev -bind "${DOCKER_GATEWAY_IP}"`
1. Ensure Nomad has started by visiting `echo "http://${DOCKER_GATEWAY_IP}:4646"`
1. Set `NOMAD_ADDR` for the Nomad CLI to access Nomad - `export NOMAD_ADDR="http://${DOCKER_GATEWAY_IP}:4646"`
1. Ensure Nomad CLI works - `nomad server members`
1. Run any job in the examples/ directory - `nomad job run examples/happy-job.hcl`
