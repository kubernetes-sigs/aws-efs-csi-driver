#!/usr/bin/env bash

# This script is used for LTE where Internet access is blocked and public docker images cannot be pulled.
# It copies images used for EFS CSI driver into Stuart registry
#

IMAGES=($(yq .images kustomization.yaml | jq -r '.[] | .name + ":" + .newTag + ";" + .newName + ":" + .newTag'))

for line in "${IMAGES[@]}";do
    PUBLIC=$(echo $line | cut -d";" -f1)
    PRIVATE=$(echo $line | cut -d";" -f2)
    docker pull $PUBLIC
    docker tag $PUBLIC $PRIVATE
    docker push $PRIVATE
done
