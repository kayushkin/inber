# Inber build commands

# Build inber-server binary
build:
  go build -o ~/bin/inber-server ./cmd/inber-server

# Run tests
test:
  go test ./...

# Install to ~/bin
install: build

# Clean
clean:
  rm -f ~/bin/inber-server
