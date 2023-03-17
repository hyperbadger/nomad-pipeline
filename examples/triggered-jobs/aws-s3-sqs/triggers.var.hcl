extra_env = {
  AWS_ACCESS_KEY_ID     = "test"
  AWS_SECRET_ACCESS_KEY = "test"
}

triggers = [
  {
    job_id  = "triggered-job"
    type    = "s3"
    trigger = {
      sqs_url  = "awssqs://localhost:4566/000000000000/triggered-job-example-queue?region=us-east-1&endpoint=localhost.localstack.cloud:4566"
      meta_key = "object_path"
    }
  }
]
