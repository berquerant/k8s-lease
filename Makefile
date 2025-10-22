BIN = bin/klock
CMD = "./cmd/klock"
THIRD_PARTY_LICENSES = NOTICE
TEST_BIN = bin/klock-incluster-test

#
# Build
#

.PHONY: $(BIN)
$(BIN):
	./bin/build.sh -o $@ $(CMD)

#
# Test
#

.PHONY: test
test: test-unit test-e2e

.PHONY: test-e2e
test-e2e: $(BIN) $(TEST_BIN) setup-cluster
	go test -race ./tests/...

.PHONY: test-unit
test-unit: setup-cluster
	go test $$(go list ./... | grep -v -E '/(lease|tests)')
	go test ./lease/ -race -ginkgo.v

.PHONY: setup-cluster
setup-cluster:
	./hack/setup-cluster.sh $(KIND_NODE_IMAGE)

.PHONY: $(TEST_BIN)
$(TEST_BIN):
	GOOS=linux ./bin/build.sh -o $@ $(CMD)

#
# Lint
#

.PHONY: lint
lint: check-licenses vet vuln golangci-lint

.PHONY: vuln
vuln:
	go tool govulncheck ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: golangci-lint
golangci-lint:
	go tool golangci-lint config verify -v
	go tool golangci-lint run

.PHONY: check-licenses-diff
check-licenses-diff: $(THIRD_PARTY_LICENSES)
	git diff --exit-code $(THIRD_PARTY_LICENSES)

.PHONY: check-licenses
check-licenses: check-licenses-diff
	./hack/license.sh check

#
# Code generation
#

.PHONY: $(THIRD_PARTY_LICENSES)
$(THIRD_PARTY_LICENSES):
	./hack/license.sh report > $@

.PHONY: generate
generate:
	go generate ./...

.PHONY: clean-generated
clean-generated:
	find . -name "*_generated.go" -type f -delete

#
# etc
#
.PHONY: clean-tools
clean-tools:
	rm -f bin/kubectl bin/kind
