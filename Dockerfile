FROM golang:1.26-bookworm AS builder
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/kling-creator-01 .

FROM chromedp/headless-shell:latest
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/kling-creator-01 /app/kling-creator-01
ENV HOST=0.0.0.0 \
    PORT=18013 \
    DATA_DIR=/data \
    DATABASE_PATH=/data/kling-creator-01.sqlite \
    CHROME_EXECUTABLE=/headless-shell/headless-shell \
    KLING_LOGIN_URL=https://klingai.com/app
VOLUME ["/data"]
EXPOSE 18013
ENTRYPOINT ["/app/kling-creator-01"]
