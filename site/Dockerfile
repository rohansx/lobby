# The landing page: static files behind nginx.
FROM nginx:1.29-alpine
COPY . /usr/share/nginx/html/
RUN mv /usr/share/nginx/html/nginx.conf /etc/nginx/conf.d/default.conf
