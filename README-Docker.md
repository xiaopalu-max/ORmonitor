# Docker 部署指南

## 快速开始

### 方式一：使用 Docker Compose（推荐）

1. **构建并启动容器**
   ```bash
   docker-compose up -d
   ```

2. **查看日志**
   ```bash
   docker-compose logs -f
   ```

3. **停止服务**
   ```bash
   docker-compose down
   ```

4. **重启服务**
   ```bash
   docker-compose restart
   ```

### 方式二：使用 Docker 命令

1. **构建镜像**
   ```bash
   docker build -t ormonitor:latest .
   ```

2. **运行容器**
   ```bash
   docker run -d \
     --name ormonitor \
     -p 3000:3000 \
     -v $(pwd)/data:/app/data \
     --restart unless-stopped \
     ormonitor:latest
   ```

3. **查看日志**
   ```bash
   docker logs -f ormonitor
   ```

4. **停止容器**
   ```bash
   docker stop ormonitor
   ```

5. **删除容器**
   ```bash
   docker rm ormonitor
   ```

## 数据持久化

数据文件存储在 `./data/db.json`，通过 Docker volume 映射到宿主机，确保容器重启后数据不丢失。

## 端口配置

默认端口为 `3000`，如需修改：

- **Docker Compose**: 修改 `docker-compose.yml` 中的端口映射 `"新端口:3000"`
- **Docker 命令**: 修改 `-p` 参数，例如 `-p 8080:3000`

## 环境变量

可以通过环境变量配置：

- `PORT`: 服务端口（默认：3000）
- `NODE_ENV`: 运行环境（默认：production）

## 访问应用

容器启动后，访问：http://localhost:3000

默认登录信息：
- 用户名：`admin`
- 密码：`admin123`

## 更新应用

1. **停止当前容器**
   ```bash
   docker-compose down
   ```

2. **重新构建镜像**
   ```bash
   docker-compose build --no-cache
   ```

3. **启动新容器**
   ```bash
   docker-compose up -d
   ```

## 故障排查

### 查看容器状态
```bash
docker ps -a
```

### 查看容器日志
```bash
docker logs ormonitor
```

### 进入容器调试
```bash
docker exec -it ormonitor sh
```

### 检查端口占用
```bash
lsof -i :3000
```
