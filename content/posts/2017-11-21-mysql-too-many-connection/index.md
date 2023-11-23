
+++
title = "MySQL Too Many Connection"
summary = ''
description = ""
categories = []
tags = []
date = 2017-11-21T14:45:34+08:00
draft = false
+++


记录一次线上事故

```Python
def get(self, request, topic=None, book=None):
    key = key_of_user_most_book(request.user.pk)
    result = cache.get(key)
    if result:
        return Result(data=result)
    half_year_ago = datetime.datetime.now() + datetime.timedelta(days=-180)
    user_book = UserBook.objects.filter(user_id=request.user.pk)
    user_book_ids = [userbook.book.id for userbook in user_book]
    most_book = UserBook.objects.all().filter(
        date__gte=half_year_ago
    ).exclude(book_id__in=user_book_ids).values(
        'book').annotate(index=Count('user')).order_by('-index')[:3]
    most_book_ids = [int(mb['book']) for mb in most_book]
    most_book = Book.objects.filter(id__in=most_book_ids)
    result = [b.manifest for b in most_book]
    cache.set(key, result)
    return Result(data=result)
```

以上代码造成了数据库 `too many connection`。原因是对于每一个用户都去查询了除自己阅读过的书以外的半年中人气最高的三本书，这是一个非常耗时的操作，导致 Django 与 Mysql 的连接过多，数据库直接被打死。
由于存在大量的 TIME_WAIT 连接，所以单凭线上项目回滚并不能解决问题，必须手动 `kill` 掉这些连接

```Bash
mysqladmin --login-path=root processlist | awk -F"|" '$2 ~ /[0-9]/ {print "KILL" $2 ";"}' | mysql --login-path=root
```

另一个问题好像是配置确实有问题，可以参考这篇文章
[通过`CONN_MAX_AGE`优化Django的数据库连接](https://www.the5fire.com/reduce-db-conn-with-django-persistent-connection.html)
    