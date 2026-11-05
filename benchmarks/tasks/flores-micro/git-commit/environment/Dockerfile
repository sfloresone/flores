FROM python:3.13-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*
COPY app/ /app/
RUN git config --global user.email "dev@example.com" && git config --global user.name "dev" && git config --global --add safe.directory /app && git init -q && git add -A && git commit -q -m "initial import"
