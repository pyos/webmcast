CXX ?= g++
CXXFLAGS ?= -O3


all: obj/tcpdump obj/webmdump


obj/%: src/%.cc lib/io.h lib/io-main.h
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -I./lib -pthread $< -o $@
