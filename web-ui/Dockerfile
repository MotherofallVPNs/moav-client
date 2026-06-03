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
# Keep the docker-compose contract — listen on 3000 inside the container.
RUN sed -i 's/listen\(.*\)80;/listen\1 3000;/' /etc/nginx/conf.d/default.conf
EXPOSE 3000
