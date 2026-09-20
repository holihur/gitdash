# 文档站便捷命令（需要已安装 Hugo extended）
HUGO ?= hugo
SRC  ?= .
PAGEFIND ?= npx --yes pagefind@1.3.0

.PHONY: serve build clean

serve:
	$(HUGO) server -D --source $(SRC) --bind 0.0.0.0

build:
	$(HUGO) --source $(SRC) --minify
	$(PAGEFIND) --site $(SRC)/public

clean:
	rm -rf $(SRC)/public $(SRC)/resources
