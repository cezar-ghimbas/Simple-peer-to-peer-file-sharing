FROM ubuntu

COPY ./servent /app/servent
COPY ./util /app/util
COPY ./message /app/message
COPY ./test_client /app/test_client
COPY ./file_generator /app/file_generator

RUN  apt-get update \
  && apt-get install -y wget \
  && rm -rf /var/lib/apt/lists/*

RUN wget -c https://golang.org/dl/go1.15.6.linux-amd64.tar.gz
RUN tar -C /usr/local -xvzf go1.15.6.linux-amd64.tar.gz
RUN rm go1.15.6.linux-amd64.tar.gz

ENV PATH "$PATH:/usr/local/go/bin"

WORKDIR /app

RUN go mod init sp2pfs

RUN go build -o ./test_client/test_client ./test_client/main.go

RUN go build -o ./file_generator/file_generator ./file_generator/main.go
RUN mkdir -p /shared
#RUN ./file_generator/file_generator /shared
