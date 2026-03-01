FROM alpine:3.20 AS certs
RUN apk add --no-cache ca-certificates tzdata

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY dkpbot /usr/local/bin/dkpbot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dkpbot"]
CMD ["--config", "/etc/dkpbot/config.yaml"]
