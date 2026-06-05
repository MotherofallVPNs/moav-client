FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

# nginx:alpine is ~50MB vs node + vite preview at ~340MB. Vite preview was
# only needed during dev; production just serves the static dist.
FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
# Custom server block: listens on 3000 (compose contract) + sets cache headers
# so a fresh build's hashed bundle is picked up instead of a stale index.html.
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
