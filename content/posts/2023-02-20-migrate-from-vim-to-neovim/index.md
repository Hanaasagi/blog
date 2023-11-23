
+++
title = "Migrate from Vim to Neovim"
summary = ''
description = ""
categories = []
tags = []
date = 2023-02-20T13:10:16+08:00
draft = false
+++

整理了一下环境，把 Vim 切换到 Neovim 了。首先如果想体验 Neovim，并不意味着你的 vimrc 要重新写，大部分是兼容的。只需要参考 [Transitioning from Vim](https://neovim.io/doc/user/nvim.html#nvim-from-vim) 中的步骤就可以基于原本在 Vim 中的配置使用 Neovim 了。我这里选择通过 Lua 重写，一是 Vimscript 这个每次要改点什么就要查语法很头疼，二是 Neovim 在当下的生态已经远比几年前好了，基本上各种 Vimscript 编写的插件都可以找到使用 Lua 重写的替代版本了

说几个奇怪的地方

- 以前保存一个没有权限的文件的时候，vim 是不能保存的。需要添加 `cmap w!! w !sudo tee > /dev/null %` 这种快捷命令。但是 Neovim 不用配置就可以做到，这个可能引起误伤
- `:shell` 没了，取而代之的是 `:terminal`。这个可能和你的 Zsh 之类的配置有冲突



## Builtin Settings

将内建的配置项和三方插件的分开后。你可以通过执行 `source` 来直接载入 VimScript 的配置，也就是说原来的基本不用动

```Lua
vim.cmd('source ' ..  'the path of VimScript')```
```

也可以重写成 Lua 版本的，这里列举几个例子

- VimScript 中的 `g` 改写成 Lua 中的 `vim.g`。比如 `let g:mapleader = ','` => `vim.g.mapleader = ","`
- 大部分的 `set x` 改写成 `vim.opt.x = value` 的形式。比如 `set ruler` => `vim.opt.ruler = true`。这里所有的选项可以通过 `:help option-list` 得到
- `autocmd` 替换成函数。比如 `au FocusLost * :set norelativenumber number` => `vim.api.nvim_create_autocmd("FocusLost", { pattern = "*", command = ":set norelativenumber number" })`。这里所用到的 `vim.api` 是用来和 vim 进行交互的。这条命令也可以进一步 Lua 化
- 按键映射方面直接使用 `vim.keymap.set` 取代，`nmap/vmap/nnoremap` 这些都被变成参数了。比如 `vim.keymap.set("n", "<leader>w", ":w<CR>", { silent = false, noremap = true, desc = "write to file" })`
- 原先的自定义函数可以重写成 Lua 版本的，然后通过 `vim.api.nvim_create_user_command` 注册成命令，这个函数的第二个参数可以为 Lua 的函数，也可以为 String 用来外部插件中的原生 VimScript 函数。比如 `vim.api.nvim_create_user_command("Inflection", "call inflection#inflect_current_word()", {})`


另外如果有问题，可能除 Help 以外， [r/neovim](https://www.reddit.com/r/neovim/) 是一个不错的选择，里面关于 Neovim 的解答可能比 stackoverflow 还多。还有一点就是可以先搜一下有没有插件实现了这个功能，Neovim 的插件极多，哪怕是一个小功能都有插件，比如高亮 TODO 的 [todo-comments](https://github.com/folke/todo-comments.nvim)



## Plugin Ecosystem

主要以 Lua 编写的插件为主。搜索的时候可以使用 Google 的搜索语法，通过双引号括起来 Neovim，来避免匹配到很多 Vim 的插件

### Plugin Manager

目前流行的应该就这两个

- [packer.nvim](https://github.com/wbthomason/packer.nvim)
- [lazy.vim](https://github.com/folke/lazy.nvim)

体验了一下感觉 lazy.nvim 更方便友好。Neovim 的默认配置目录，在 `~/.config/nvim` 中。如果使用 lazy.nvim 可以通过配置来自动载入
`~/.config/nvim/lua/plugins.lua` 或者 `~/.config/nvim/lua/plugins/*.lua` 中的内容。所以你可以将配置目录整理成这样


```
.
├── init.lua
├── lazy-lock.json
├── lua
│   └── plugins
│       ├── completion.lua
│       ├── finder.lua
│       ├── lsp.lua
│       ├── misc.lua
│       ├── operation.lua
│       ├── syntax.lua
│       └── ui.lua
```


`init.lua` 写入包管理器所需的必要代码

```Lua
local _M = {}

function _M.setup()
  -- load plugin manager
  local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"
  if not vim.loop.fs_stat(lazypath) then
    print('installing lazy.nvim...')
    vim.fn.system({
      "git",
      "clone",
      "--filter=blob:none",
      "https://github.com/folke/lazy.nvim.git",
      "--branch=stable", -- latest stable release
      lazypath,
    })
    print('installing lazy.nvim...done')
  end
  vim.opt.rtp:prepend(lazypath)

  -- load plugins, auto import lua/plugins files
  require("lazy").setup({ { import = "plugins" } })
end

_M.setup()
```

在 `~/.config/nvim/lua/plugins/*.lua` 的每个文件中，返回一个配置插件的 `table` 就可以了。参考如下

```Lua
-- ~/.config/nvim/lua/plugins/completion.lua`
return {
  -- https://github.com/windwp/nvim-autopairs
  -- A super powerful autopair plugin for Neovim that supports multiple characters.
  {
    "windwp/nvim-autopairs",
    config = function()
      require("nvim-autopairs").setup()
    end,
  },
}
```

上面代码中的 `config` 用于配置插件，需要视插件本身而定。lazy.nvim 本身也提供了很多配置参数，但用下来基本只需要关注 `event` 参数，用来控制载入的时机


### Some Useful Plugins

这一节主要列举一些可以替换的插件。首先可以先装 [neodev.nvim](https://github.com/folke/neodev.nvim) 这个插件，可以为编写 Neovim 配置提供环境，支持文档，跳转

#### Language Server

依旧可以选择 [coc.nvim](https://github.com/neoclide/coc.nvim) 一套带走。我这里则是基于原生的生态来改

首先是 Language Server Manager，可以帮助我们对 LS 进行管理

- [mason.nvim](https://github.com/williamboman/mason.nvim)
- [mason-lspconfig.nvim](https://github.com/williamboman/mason-lspconfig.nvim)
- [nvim-lspconfig](https://github.com/neovim/nvim-lspconfig)

另外还推荐 

- [null-ls.nvim](https://github.com/jose-elias-alvarez/null-ls.nvim)
- [mason-null-ls.nvim](https://github.com/jay-babu/mason-null-ls.nvim)

这可以使用额外的工具作为数据源来做一些代码重命名，格式化之类的，弥补 Neovim 内置的 LSP 的不足


- [lspsaga.nvim](https://github.com/glepnir/lspsaga.nvim) 实现代码内跳转，查找调用等功能
- [nvim-cmp](https://github.com/hrsh7th/nvim-cmp) 这只是一个补全引擎，需要其他的插件为其提供输入源，比如 cmp-nvim-lsp
- [cmp-nvim-lsp](https://github.com/hrsh7th/cmp-nvim-lsp) 基于 lsp 的补全
- [cmp-buffer](https://github.com/hrsh7th/cmp-buffer) 提供基于 Buffer 中内容的补全项
- [cmp-path](https://github.com/hrsh7th/cmp-path)可以提供系统路径的补全项
- [cmp-cmdline](https://github.com/hrsh7th/cmp-cmdline) 提供 Vim cmd 命令的补全
- [cmp-cmdline-history](https://github.com/dmitmel/cmp-cmdline-history) 命令历史的补全
- [lspkind.nvim](https://github.com/onsails/lspkind.nvim) 美化补全列表用的，可以选择不装



#### Fuzzy Finder

用于模糊查找，可以用于替换 LeaderF

- [telescope.nvim](https://github.com/nvim-telescope/telescope.nvim)

这一个系列也有好几个插件需要安装，可以自行 Google

#### Syntax Enhanced

- [nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter)

用处之一是可以增强高亮显示，不再需要像 Vim 那样去装 rust-lang/rust.vim 这种东西了。除此之外还可以为其他的插件提供语义支持，比如可以 block 级别的跳转

可以考虑以下插件进行配合

- [nvim-ts-autotag](https://github.com/windwp/nvim-ts-autotag) 补全 html tag 
- [nvim-autopairs](https://github.com/windwp/nvim-autopairs) 补全括号的
- [vim-matchup](https://github.com/andymass/vim-matchup) 支持 block 级别跳转，比如 `if` 跳转到 `elif`
- [nvim-treesitter-context](https://github.com/nvim-treesitter/nvim-treesitter-context) 显示上下文信息，用于函数太长的时候
- [nvim-treesitter-textobjects](https://github.com/nvim-treesitter/nvim-treesitter-textobjects) 基于 treesitter 做跳转，选中等，支持功能比较多，部分和 vim-matchup 重叠


#### Snippets

看个人喜好，我这里使用的是当前 star 数最多的 LuaSnip

- [L3MON4D3/LuaSnip](https://github.com/L3MON4D3/LuaSnip) Snippets Engine
- [cmp_luasnip](https://github.com/saadparwaiz1/cmp_luasnip) 缝合 nvim-cmp 和 LuaSnip 的插件，可以在补全列表中提供 Snippets 方面的补全项
- [friendly-snippets](https://github.com/rafamadriz/friendly-snippets) 各种语言的 Snippets 集合

#### Ohters
一些其他的插件，用于增强体验

- [hop.nvim](https://github.com/phaazon/hop.nvim) 用于快速查找匹配的字母并且跳转，增强 Neovim 内置的 `f/F` 等功能。很好用建议装
- [leap.nvim](https://github.com/ggandor/leap.nvim) 和 hop.nvim 类似，二选一
- [Comment.nvim](https://github.com/numToStr/Comment.nvim) 为注释提供快捷操作，可以配合 [nvim-ts-context-commentstring](JoosepAlviste/nvim-ts-context-commentstring) 一起使用
- [nvim-surround](https://github.com/kylechui/nvim-surround) 对于周围括号/引号等符号进行成对操作
- [NeoZoom.lua](https://github.com/nyngwang/NeoZoom.lua) 用于放大/浮动当前窗口
- [gitsigns.nvim](https://github.com/lewis6991/gitsigns.nvim) 增加 Git 相关的支持。目前感觉还是 [vim-fugitive](https://github.com/tpope/vim-fugitive) 好一些
- [nvim-hlslens](https://github.com/kevinhwang91/nvim-hlslens) 增强 Vim 的搜索，提供高亮还有基于 virtual text 的搜索位置提示


关于插件启动速度，可以使用 Lazy.nvim 的中提供的 Profile 功能。如果遇到插件安装后没有反应，可以试着执行 `:checkhealth` 这个命令，里面会有详细信息，前提插件实现了健康检查的接口

    