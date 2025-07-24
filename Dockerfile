# Stage 1: The Builder (This stage is correct and remains the same)
FROM nvidia/cuda:12.2.2-devel-ubuntu22.04 AS builder

RUN apt-get update && apt-get install -y wget && \
    wget https://golang.org/dl/go1.22.5.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz && \
    rm go1.22.5.linux-amd64.tar.gz

RUN apt-get update && apt-get install -y \
    build-essential \
    wget \
    smartmontools \
    && rm -rf /var/lib/apt/lists/*

ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /src
COPY ./go-agent/ .

RUN CGO_ENABLED=1 GOOS=linux go build -o /unified-agent .


# --- Stage 2: The Final, Compatible Runtime Image ---

# --- THIS IS THE FIX ---
# We now use a slim ubuntu image which is compatible with the glibc
# library used in the builder stage.
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y smartmontools && rm -rf /var/lib/apt/lists/*


# Copy the compiled binary from the absolute path in the builder
COPY --from=builder /unified-agent /unified-agent

# Add execute permissions
RUN chmod +x /unified-agent

# Run the command using the absolute path
CMD ["/unified-agent"]