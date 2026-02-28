FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ARG BINARY=dkpbot
COPY ${BINARY} /usr/local/bin/dkpbot
EXPOSE 8080
ENTRYPOINT ["dkpbot"]
CMD ["--config", "/etc/dkpbot/config.yaml"]
