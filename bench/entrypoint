#!/bin/bash

ZONE_NAME=$(curl -s $ECS_CONTAINER_METADATA_URI_V4/task | jq -r .AvailabilityZone)
ZONE_ID=$(aws ec2 describe-availability-zones \
	| jq -r ".AvailabilityZones[] | select(.ZoneName==\"${ZONE_NAME}\").ZoneId")
export ISUXPORTAL_SUPERVISOR_INSTANCE_NAME="${ZONE_ID}"

exec "$@"
