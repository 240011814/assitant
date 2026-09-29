FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV TZ=Asia/Shanghai
# 容器内必须监听 0.0.0.0, 端口映射才可达(shared.py 读该变量); 容器外暴露面由 -p 端口映射决定
ENV BAOSTOCK_API_HOST=0.0.0.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends tzdata \
    && ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
    && echo $TZ > /etc/timezone \
    && rm -rf /var/lib/apt/lists/*

ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG PIP_TRUSTED_HOST=pypi.tuna.tsinghua.edu.cn

WORKDIR /app

COPY requirements.txt ./

RUN python -m pip install --no-cache-dir --upgrade pip \
    && python -m pip install --no-cache-dir \
      --index-url ${PIP_INDEX_URL} \
      --trusted-host ${PIP_TRUSTED_HOST} \
      -r requirements.txt

COPY pyproject.toml ./
COPY main.py ./
COPY baostock_api ./baostock_api

# 非 root 用户运行; data 目录(usage.json)需要写权限
RUN mkdir -p /app/data \
    && useradd --create-home --uid 10001 baostock \
    && chown -R baostock:baostock /app

USER baostock

EXPOSE 3002

# 镜像无 curl, 用标准库 urllib 探活本机 /health
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request,os;urllib.request.urlopen('http://127.0.0.1:'+os.environ.get('BAOSTOCK_API_PORT','3002')+'/health')"

CMD ["python", "main.py"]
