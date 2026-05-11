#!/bin/bash
set -e

# This script uses Infracost to estimate the cost of the test cluster.
# You must have infracost installed and configured.

echo "Calculating cost estimate for the test cluster..."

cd terraform
infracost breakdown --path . \
    --usage-file ../infracost-usage.yml \
    --format table

echo ""
echo "Note: This is an estimate based on the current Terraform plan."
