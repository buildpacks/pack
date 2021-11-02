FROM docker.io/library/golang:1.16 AS builder

RUN mkdir /workspace/
COPY main.go go.mod /workspace/
WORKDIR /workspace/
ENV CGO_ENABLED=0
RUN go build -o serve-socket

FROM fedora:33

RUN dnf in -y openssh-server passwd socat

COPY etc/ssh /etc/ssh
RUN chmod og-rwx /etc/ssh/*_key
COPY entrypoint.sh /usr/local/bin
RUN chmod a+x /usr/local/bin/entrypoint.sh

RUN groupadd testuser && \
    useradd testuser -g testuser -m -s /bin/bash && \
    echo iddqd | passwd root --stdin && \
    echo idkfa | passwd testuser --stdin

RUN su testuser && mkdir /home/testuser/.ssh/
COPY --chown=testuser:testuser id_ed25519.pub id_rsa.pub /tmp/
RUN cat /tmp/id_ed25519.pub /tmp/id_rsa.pub >> /home/testuser/.ssh/authorized_keys

COPY --from=builder /workspace/serve-socket /usr/local/bin

EXPOSE 22

CMD ["/usr/local/bin/entrypoint.sh"]
