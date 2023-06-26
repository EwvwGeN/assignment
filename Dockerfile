FROM alpine
ADD main /server/
ADD ./configs/ /configs/
ARG ISCNF
ENV ISCNF=${ISCNF}
CMD ["sh", "-c", "./server/main $ISCNF"]
