FROM golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b
WORKDIR /app
COPY backend/ ./
RUN go build -trimpath -o /usr/local/bin/podolyam ./cmd/server
EXPOSE 8080
CMD ["podolyam"]
