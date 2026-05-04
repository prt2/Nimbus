# Install the React dependencies and run the development server on port 3000.

FROM node:22-alpine

WORKDIR /app

# Install the exact versions recorded in the lockfile before copying application source.
COPY package.json package-lock.json ./
RUN npm ci

COPY public ./public
COPY src ./src

# Listen on all container interfaces so the published host port can reach the server.
ENV HOST=0.0.0.0
ENV PORT=3000

EXPOSE 3000

CMD ["npm", "start"]
