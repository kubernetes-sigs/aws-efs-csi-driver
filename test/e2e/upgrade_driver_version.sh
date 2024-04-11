#!/bin/sh
set -eux

# Check for dependencies
if ! command -v kubectl &> /dev/null; then
  echo "kubectl is not installed."
  exit 1
fi
if ! command -v docker &> /dev/null; then
  echo "Docker is not installed."
  exit 1
fi
if ! command -v aws &> /dev/null; then
  echo "AWS CLI is not installed."
  exit 1
fi

prevRelease=$1
region=$2
account=$3

# The private ECR Repo is expected to take the following format:
# $account.dkr.ecr.$region.amazonaws.com/aws-efs-csi-driver
ecrRegistry="$account.dkr.ecr.$region.amazonaws.com"
ecrRepo="aws-efs-csi-driver"

# Make temp folder for temp files
mkdir ./temp
publicDriverManifest="./temp/public-driver-manifest.yaml"

# Build & push image of driver's development version
cd ../.. && make 
aws ecr get-login-password --region $region | docker login --username AWS --password-stdin $ecrRegistry
docker build --pull --no-cache -t aws-efs-csi-driver .
docker tag aws-efs-csi-driver:latest $ecrRegistry/$ecrRepo:latest
docker push $ecrRegistry/$ecrRepo:latest
cd ./test/e2e

# Download starting version manifest
kubectl kustomize \
    "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/?ref=release-$prevRelease" > $publicDriverManifest

# Remove the predefined service accounts
awk '
  /apiVersion: v1/ {recording=1} 
  recording && /app.kubernetes.io\/name: aws-efs-csi-driver/ {found=1} 
  recording && /name: efs-csi-controller-sa/ {controller_sa=1} 
  recording && /name: efs-csi-node-sa/ {node_sa=1} 
  recording && /---/ {
    if (found && (controller_sa || node_sa)) {
      recording=0; found=0; controller_sa=0; node_sa=0; next
    }
  }
  !recording {print}
' $publicDriverManifest > ./temp/temp.yaml && mv ./temp/temp.yaml $publicDriverManifest

# Deploy starting version of driver for the upgrade test
kubectl apply -f $publicDriverManifest

# Tear down starting version
kubectl delete -f $publicDriverManifest

# Create private manifest file & modify to use the private ecr repo
privateDriverManifest="./temp/private-driver-manifest.yaml"
kubectl kustomize \
    "github.com/kubernetes-sigs/aws-efs-csi-driver/deploy/kubernetes/overlays/stable/ecr/?ref=release-$prevRelease" > $privateDriverManifest

sed -i -e "s|602401143452.dkr.ecr.us-west-2.amazonaws.com/eks/aws-efs-csi-driver:v[0-9]\+\.[0-9]\+\.[0-9]\+|$account.dkr.ecr.$region.amazonaws.com/aws-efs-csi-driver:latest|" $privateDriverManifest

# Remove the predefined service accounts
awk '
  /apiVersion: v1/ {recording=1} 
  recording && /app.kubernetes.io\/name: aws-efs-csi-driver/ {found=1} 
  recording && /name: efs-csi-controller-sa/ {controller_sa=1} 
  recording && /name: efs-csi-node-sa/ {node_sa=1} 
  recording && /---/ {
    if (found && (controller_sa || node_sa)) {
      recording=0; found=0; controller_sa=0; node_sa=0; next
    }
  }
  !recording {print}
' $privateDriverManifest > ./temp/temp.yaml && mv ./temp/temp.yaml $privateDriverManifest

kubectl apply -f $privateDriverManifest

rm -rf ./temp