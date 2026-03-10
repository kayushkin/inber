# Inber build commands

# Build inber binary
build:
  go build -o ~/bin/inber ./cmd/inber

# Run tests
test:
  go test ./...

# Install to ~/bin
install: build

# Clean
clean:
  rm -f ~/bin/inber
