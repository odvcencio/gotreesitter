variable "region" {
  type    = string
  default = "us-east-1"
}

resource "aws_s3_bucket" "corpus" {
  bucket = "depth-census-${var.region}"
  tags = {
    owner = "census"
    tier  = "evidence"
  }
}
