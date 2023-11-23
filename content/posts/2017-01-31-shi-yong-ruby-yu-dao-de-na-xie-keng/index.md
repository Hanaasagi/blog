
+++
title = "使用 Ruby 遇到的那些坑"
summary = ''
description = ""
categories = []
tags = []
date = 2017-01-31T13:49:40+08:00
draft = false
+++

第一次尝试 Ruby 开发，写了 400+ lines 代码，硬是坑了自己一天

诚然，是自己学艺不精

说一说，今天到底遇到了什么坑
##### 1) 数组
Ruby 中的数组切片是 `[begin..end]`， 在代码中有一处写成了 `[index, count]` 将近半小时才 debug 出来
还有更坑爹的一点 切片操作是包含尾部边界的
#####2) strip
Ruby 的字符串方法 `strip` 并不能去除指定的字符
查找 stackoverflow 的解决方案后，用往 String 类加了扩展

    module StringExtensions
      refine String do
        def strip(chars=nil)
          if chars
            chars = Regexp.escape(chars)
            self.gsub(/\A[#{chars}]+|[#{chars}]+\z/, "")
          else
            super()
          end
        end
      end
    end

用了 `refinement` ，避免 `monkeypatch`

##### 3) 字符串定义
定义字符串有三种
`''` 不会转义，写 Python 写惯了，不适应
`""` 转义
`<<~HEREDOC
HEREDOC`

##### 4) puts 和 print
debug 做断点时遇到的， `puts` 会将 `\n` 这样的字符直接当换行输出

    irb(main):006:0> arr = ["\n"]
    => ["\n"]
    irb(main):007:0> puts arr

    => nil
    irb(main):008:0> print arr
    ["\n"]=> nil

##### 5) 内部类
脑子真是抽风了，还写出了这种代码

    class Template
      def self.render
        new.render
      end

      def common
        puts 'common methods'
      end

      def render
        CondTemplate.render
        # Looptemplate.render
        # ...
      end

      class CondTemplate
        def self.render
          common
        end
      end
    end

    Template.render

Output

    test.rb:19:in `render': undefined local variable or method `common' for Template::CondTemplate:Class (NameError)

`CondTemplate` 是定义在 `Template` 内部，所以应该可以访问其方法。想想好像没有什么问题，平常又不是没写过闭包

    def closure
      def inner_func
        puts 'this is inner'
      end

      def pass
        inner_func()
      end
      method(:pass)
    end

    func = closure()
    func.call

但是仔细想想，好像有点问题。于是用 Python 写了一遍，发现也会报错。但是 Java 是可以的

    import java.io.*;
    class OuterClass {

        class InnerClass{
            public String innerFunc(){
                return outerFunc();
            }
        }

        public String outerFunc(){
            return "this is outer";
        }
    }


    class Test{

        public static void main(String args[]) {

            OuterClass outer = new OuterClass();
            OuterClass.InnerClass inner = outer.new InnerClass();

            System.out.println(inner.innerFunc());
        }
    }

根据一番 Google，这样写其实是为了完善多继承。而 Java 可以这样写，好像是因为原来没有 `lambda` `closure` 这些，又不能多继承。所以为了增强 Java 的使用，引入了内部类来弥补。而 Python 是支持多继承的， Ruby 也有 Mixin 的对策。
详细的戳这里 [Java 中引入内部类的意义？](https://www.zhihu.com/question/21373020)
**回头要深究一下这个问题**
#####6) 函数参数
Ruby 不支持这种写法

    def func1(a, b=1, c=2, d=3)
      puts "a=#{a}"
      puts "b=#{b}"
      puts "c=#{c}"
      puts "d=#{d}"
    end

    def func2(*args, **kwargs)
      func1(*args, **kwargs)
    end

    func2(10, 20, c=100, d=1000)

`**` 不会 `unpack` 到相应的参数中去
[如何以正确的姿势传一个 hash 参数](https://ruby-china.org/topics/20622)
 不仅仅是这样

    func1(1, c=100)

Output

    a=1
    b=100
    c=2
    d=3

替代方法

    def func1(a, b:1, c:2, d:3)
      puts "a=#{a}"
      puts "b=#{b}"
      puts "c=#{c}"
      puts "d=#{d}"
    end

    func1(1, c:100)

但是这样不能这样传参  `func1(1, 2, c:3, d:4)`
[Ruby 有没有可能跳过一个带默认值的参数，为下一个带默认值的参数提供值？](https://ruby-china.org/topics/9782)


