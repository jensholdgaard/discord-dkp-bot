FROM alpine:3.20@sha256:a4f4213abb84c497377b8544c81b3564f313746700372ec4fe84653e4fb03805 AS certs
RUN apk add --no-cache ca-certificates tzdata

FROM scratch

ARG USER_UID=10001
ARG USER_GID=10001

USER ${USER_UID}:${USER_GID}

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --chmod=755 dkpbot /usr/local/bin/dkpbot

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/dkpbot"]
CMD ["--config", "/etc/dkpbot/config.yaml"]
