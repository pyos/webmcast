AR ?= ar
CXX ?= g++
CXXFLAGS ?= -O3


.PHONY: all clean


all: obj/libwebmwrap.a
clean:
	rm -rf obj webmffi.*


obj/webmwrap.o: webmwrap.cc webmwrap.h webm.cc
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -fPIC $< -c -o obj/webmwrap.o


obj/libwebmwrap.a: obj/webmwrap.o
	$(AR) rcs $@ obj/webmwrap.o
