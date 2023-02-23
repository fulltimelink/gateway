FROM alpine:3.17.1
LABEL maintainer="120608668@qq.com"
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# set +8
RUN apk --no-cache add bash ca-certificates openssl curl tzdata mailcap htop sysstat procps \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# alpine testing package
RUN apk add --no-cache --repository https://mirrors.aliyun.com/alpine/edge/testing duf --allow-untrusted
###############################################################################
#                                INSTALLATION
###############################################################################

# 设置固定的项目路径
ENV WORKDIR /var/www/gateway

# 添加应用可执行文件，并设置执行权限
ADD ./gateway   $WORKDIR/gateway
RUN chmod +x $WORKDIR/*
VOLUME $WORKDIR/configs

ENV SHELL /bin/bash
WORKDIR $WORKDIR
EXPOSE 8080
EXPOSE 7070
CMD ["./gateway", "-conf", "configs/config.yaml", "-discovery.dsn", "nacos://nacos.java:8848?namespaceid=dx-transcode&timeout=5000&loglevel=debug&notloadcacheatstart=true"]