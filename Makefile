API_PROTO_FILES=$(shell find api -name *.proto)

.PHONY: api
# generate api proto
api:
	protoc --proto_path=./api \
 	       --go_out=paths=source_relative:./api \
	       $(API_PROTO_FILES)

.PHONY: build
build:
	GOOS=linux GOARCH=amd64 go build -o gateway github.com/go-kratos/gateway/cmd/gateway

versions := 0.1.0
.PHONY: docker
docker:
	docker build -t gateway:v$(versions) .
	docker tag gateway:v$(versions) hub-tx.dianzhenkeji.com/fulltimelink/gateway:v$(versions)
	docker login https://hub-tx.dianzhenkeji.com/
	docker push hub-tx.dianzhenkeji.com/fulltimelink/gateway:v$(versions)