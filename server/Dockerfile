# Production Dockerfile for Search Server
FROM node:20-bookworm-slim AS base

# Install build dependencies for better-sqlite3
RUN apt-get update && apt-get install -y \
    python3 \
    make \
    g++ \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy package files
COPY package.json package-lock.json* ./

# Install production dependencies
RUN npm ci --omit=dev

# Copy source code
COPY . .

# Create required directories
RUN mkdir -p logs data

# Expose port
EXPOSE 1213

# Environment defaults
ENV NODE_ENV=production
ENV PORT=1213

# Start the application
CMD ["node", "server.js"]
