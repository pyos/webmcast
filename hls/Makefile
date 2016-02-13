NGINX_VERSION ?= 1.9.10
NGINX = nginx-$(NGINX_VERSION)


STREAM ?= a


what:
	@echo make what


run: nginx
	./$(NGINX)/objs/nginx -p . -c nginx.conf


stream:
	sudo ffmpeg -f video4linux2 -i /dev/video0 -c:v libx264 -f flv rtmp://localhost:1935/stream/$(STREAM)


nginx: $(NGINX)/objs/nginx


nginx-rtmp-module/.git: ../.gitmodules
	git submodule update --init nginx-rtmp-module


$(NGINX):
	curl -LO http://nginx.org/download/$(NGINX).tar.gz
	tar xf $(NGINX).tar.gz
	rm $(NGINX).tar.gz


$(NGINX)/objs/nginx: $(NGINX) nginx-rtmp-module/.git
	cd $(NGINX) && ./configure --add-module=../nginx-rtmp-module --with-http_ssl_module
	make -C $(NGINX)
