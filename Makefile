GO111MODULE    = on
OPERATOR_NAME  = cloudflare-operator
CONTAINER     ?= ghetzel/cloudflare-operator:latest

.EXPORT_ALL_VARIABLES:

all: crds
	operator-sdk build $(CONTAINER)

run:
	operator-sdk run local --watch-namespace=''

regen:
	operator-sdk generate k8s

crds:
	operator-sdk generate crds