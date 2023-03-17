# Triggers

Trigger allow you to create jobs and pipelines that run as a result of an event in an external system. Your job must be a parameterized job for this to work. Inputs are supported through both meta keys and payload mechanisms that are built into Nomad.

This is achieved through the server component which can listen to many pub sub services like AWS SQS or Azure Service Bus.

Supported triggers:

* `s3` - S3 events sent using bucket notifications into an SQS queue
* `simplepubsub` - JSON messages in any pubsub service supported by Go Cloud

## `s3` - S3 events using bucket notifications

Have jobs that run on the creation or update of S3 objects. This is achieved through bucket notifications that is configured on your S3 bucket to send events to an SQS queue.

### Options

* `sqs_url` `(string: required)` - 
* `meta_key` `(string: required)` - 
* `bucket_url` `(string: "")` -
* `settings_ext` `(string: ".yaml")` -
* `object_filter` `(string: ".*")` - 
* `ack_no_match` `(bool: false)` -

### Examples and use cases

#### Running the example

There is a fully working example using `localstack` under `examples/triggered-jobs/aws-s3-sqs`. After `localstack` is up and running, see [the guide](./contributing.md#using-localstack-for-developing-and-testing-features-with-aws-dependencies), follow the steps below:

1. Deploy the parametrized job which will be triggered by the s3 create events - `nomad job run examples/triggered-jobs/aws-s3-sqs/triggered-job.hcl`
1. Create a test s3 bucket - `awslocal s3 mb s3://test-bucket`
1. Create the queue where the s3 bucket notification will be sent to - `awslocal sqs create-queue --queue-name triggered-job-example-queue`
1. Apply the bucket notification configuration to the s3 bucket - `awslocal s3api put-bucket-notification-configuration --bucket test-bucket --notification-configuration file://examples/triggered-jobs/aws-s3-sqs/bucket-notification-config.json`
1. Deploy the nomad pipeline server with the new trigger configuration - `nomad job run -var-file examples/triggered-jobs/aws-s3-sqs/triggers.var.hcl deploy/server.hcl`
1. You should now be able to upload a file in s3 and see that an instance of the parameterized job is created, here I am just using the bucket configuration as a test file for uploading - `awslocal s3 cp ./examples/triggered-jobs/aws-s3-sqs/bucket-notification-config.json s3://test-bucket/`

## `simplepubsub` - Generic JSON messages on any supported pubsub service

### Options

* `pubsub_url` `(string: required)` - 
