CC ?= gcc
CXX ?= g++
CFLAGS ?= -O3
CXXFLAGS ?= -O3


.PHONY: all
.PRECIOUS: obj/%


all: obj/tcpdump obj/webmdump obj/webmclone


obj/%: src/%.cc lib/io.h lib/io-main.h lib/ebml.h obj/picohttpparser.o
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -I./picohttpparser -I./lib -pthread $< -o $@ obj/picohttpparser.o


obj/picohttpparser.o: picohttpparser/.git
	@mkdir -p obj
	$(CC) $(CFLAGS) -std=c11 -Wall -Wextra -Werror -I./picohttpparser picohttpparser/picohttpparser.c -o $@ -c


picohttpparser/.git: ../.gitmodules
	git submodule update --init picohttpparser
