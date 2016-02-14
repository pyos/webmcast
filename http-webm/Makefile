AR ?= ar
CXX ?= g++
CXXFLAGS ?= -O3


.PHONY: all clean


all: obj/libbroadcast.a


clean:
	rm -rf obj webmffi.*


obj/%.o: %.cc %.h
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -fPIC $< -c -o $@


obj/libbroadcast.a: obj/broadcast.o
	$(AR) rcs $@ $<
