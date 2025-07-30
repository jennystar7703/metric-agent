# =================================================================
# BUILD STAGE FOR GPU
# =================================================================
FROM nvidia/cuda:12.2.2-devel-ubuntu22.04 AS gpu_builder

RUN apt-get update && apt-get install -y wget && \
    wget https://golang.org/dl/go1.23.7.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.23.7.linux-amd64.tar.gz && \
    rm go1.23.7.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update && apt-get install -y build-essential smartmontools && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY ./go-agent/ .
RUN CGO_ENABLED=1 GOOS=linux go build -tags=gpu -o /unified-agent-gpu .


# =================================================================
# BUILD STAGE FOR CPU
# =================================================================
FROM golang:1.23.7 AS cpu_builder

RUN apt-get update && apt-get install -y build-essential smartmontools && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY ./go-agent/ .
RUN CGO_ENABLED=1 GOOS=linux go build -o /unified-agent-cpu .


# =================================================================
# FINAL IMAGE FOR GPU
# =================================================================
FROM nvidia/cuda:12.2.2-base-ubuntu22.04 AS gpu
RUN apt-get update && apt-get install -y smartmontools && rm -rf /var/lib/apt/lists/*
COPY --from=gpu_builder /unified-agent-gpu /unified-agent
RUN chmod +x /unified-agent
CMD ["/unified-agent"]

# =================================================================
# FINAL IMAGE FOR CPU
# =================================================================
FROM ubuntu:22.04 AS cpu
RUN apt-get update && apt-get install -y smartmontools && rm -rf /var/lib/apt/lists/*
COPY --from=cpu_builder /unified-agent-cpu /unified-agent
RUN chmod +x /unified-agent
CMD ["/unified-agent"]
