# 文档站便捷命令（需要已安装 Hugo extended）
HUGO ?= hugo
SRC  ?= .

.PHONY: serve build clean

serve:
	$(HUGO) server -D --source $(SRC) --bind 0.0.0.0

build:
	$(HUGO) --source $(SRC) --minify

clean:
	rm -rf $(SRC)/public $(SRC)/resources
