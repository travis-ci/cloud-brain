FROM matteofigus/docker-sqitch

ADD . /sqitch

WORKDIR /sqitch

CMD sleep 7; sqitch deploy -t docker_cloudbrain
