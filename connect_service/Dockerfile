FROM ubuntu

COPY ./connect_service /app/connect_service
COPY ./common /app/common

RUN  apt-get update \
  && apt-get install -y wget \
  && rm -rf /var/lib/apt/lists/*

RUN wget -c https://golang.org/dl/go1.15.6.linux-amd64.tar.gz
RUN tar -C /usr/local -xvzf go1.15.6.linux-amd64.tar.gz
RUN rm go1.15.6.linux-amd64.tar.gz

ENV PATH "$PATH:/usr/local/go/bin"

WORKDIR /app

RUN go mod init sp2pfs

RUN go build -o ./connect_service/connect_service ./connect_service/main.go