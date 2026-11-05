FROM python:3.13-slim
WORKDIR /app
COPY app/gen_log.py /tmp/gen_log.py
RUN mkdir -p /app/logs && python3 /tmp/gen_log.py && rm /tmp/gen_log.py
COPY app/ /app/
