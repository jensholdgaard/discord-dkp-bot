FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY dkpbot /usr/local/bin/dkpbot
EXPOSE 8080
ENTRYPOINT ["dkpbot"]
CMD ["--config", "/etc/dkpbot/config.yaml"]
