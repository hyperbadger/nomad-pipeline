# Triggers

Supported triggers:

* `s3` - S3 events sent using bucket notifications into an SQS queue
* `simplepubsub` - JSON messages in any pubsub service supported by Go Cloud

## `s3` - S3 events using bucket notifications

### Options

* `sqs_url` `(string: required)` - 
* `meta_key` `(string: required)` - 
* `bucket_url` `(string: "")` -
* `settings_ext` `(string: ".yaml")` -
* `object_filter` `(string: ".*")` - 
* `ack_no_match` `(bool: false)` -
