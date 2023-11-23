
+++
title = "Tornado secure cookie"
summary = ''
description = ""
categories = []
tags = []
date = 2017-04-10T13:01:49+08:00
draft = false
+++

*本篇文章分析 Tornado 中 secure cookie 的实现，代码基于 Tornado 4.4.3*

关于 secure cookie 的使用请参考[文档](http://www.tornadoweb.org/en/stable/guide/security.html)

Tornado 中的 secure cookie 是通过 `set_secure_cookie` 和 `get_secure_cookie` 方法对 cookie 进行签名来实现的，主要是用来防止篡改，保证其完整性但并不保证私密性。 使用这两个方法的前提是在 app 中传入的 settings 中包含名为 `cookie_secure` 的项。

`set_secure_cookie` 存在两种版本，现在默认使用版本2

下面是摘录的部分源代码

```python
# tornado/web.py
class RequestHandler(object):

    # ...

    def set_secure_cookie(self, name, value, expires_days=30, version=None,
                      **kwargs):
        """Signs and timestamps a cookie so it cannot be forged.

        You must specify the ``cookie_secret`` setting in your Application
        to use this method. It should be a long, random sequence of bytes
        to be used as the HMAC secret for the signature.

        To read a cookie set with this method, use `get_secure_cookie()`.

        Note that the ``expires_days`` parameter sets the lifetime of the
        cookie in the browser, but is independent of the ``max_age_days``
        parameter to `get_secure_cookie`.

        Secure cookies may contain arbitrary byte values, not just unicode
        strings (unlike regular cookies)

        .. versionchanged:: 3.2.1

           Added the ``version`` argument.  Introduced cookie version 2
           and made it the default.
        """
        self.set_cookie(name, self.create_signed_value(name, value,
                                                       version=version),
                        expires_days=expires_days, **kwargs)

    def create_signed_value(self, name, value, version=None):
        """Signs and timestamps a string so it cannot be forged.

        Normally used via set_secure_cookie, but provided as a separate
        method for non-cookie uses.  To decode a value not stored
        as a cookie use the optional value argument to get_secure_cookie.

        .. versionchanged:: 3.2.1

           Added the ``version`` argument.  Introduced cookie version 2
           and made it the default.
        """
        # 如果 self.application.settings
        # 不存在 cookie_secret 或者 secure cookies 的键则会抛出异常
        self.require_setting("cookie_secret", "secure cookies")
        secret = self.application.settings["cookie_secret"]
        # secret 如果为一个 dict 才会用到这个变量，作为键来取 secret 中对应的 value
        key_version = None
        if isinstance(secret, dict):
            if self.application.settings.get("key_version") is None:
                raise Exception("key_version setting must be used for secret_key dicts")
            key_version = self.application.settings["key_version"]

        return create_signed_value(secret, name, value, version=version,
                                   key_version=key_version)

    # ...

def create_signed_value(secret, name, value, version=None, clock=None,
                       key_version=None):
    if version is None:
        version = DEFAULT_SIGNED_VALUE_VERSION　# 全局变量值为 2
    if clock is None:
        clock = time.time

    # utf8 在 tornado/escape.py 中定义，会将 text 转成 utf8 如果为 bytes 则直接返回
    timestamp = utf8(str(int(clock())))
    value = base64.b64encode(utf8(value))
    if version == 1:
        signature = _create_signature_v1(secret, name, value, timestamp)
        value = b"|".join([value, timestamp, signature])
        return value
    elif version == 2:
        # The v2 format consists of a version number and a series of
        # length-prefixed fields "%d:%s", the last of which is a
        # signature, all separated by pipes.  All numbers are in
        # decimal format with no leading zeros.  The signature is an
        # HMAC-SHA256 of the whole string up to that point, including
        # the final pipe.
        #
        # The fields are:
        # - format version (i.e. 2; no length prefix)
        # - key version (integer, default is 0)
        # - timestamp (integer seconds since epoch)
        # - name (not encoded; assumed to be ~alphanumeric)
        # - value (base64-encoded)
        # - signature (hex-encoded; no length prefix)
        def format_field(s):
            return utf8("%d:" % len(s)) + utf8(s)
        to_sign = b"|".join([
            b"2",
            format_field(str(key_version or 0)),
            format_field(timestamp),
            format_field(name),
            format_field(value),
            b''])

        if isinstance(secret, dict):
            assert key_version is not None, 'Key version must be set when sign key dict is used'
            assert version >= 2, 'Version must be at least 2 for key version support'
            secret = secret[key_version]

        signature = _create_signature_v2(secret, to_sign)
        return to_sign + signature
    else:
        raise ValueError("Unsupported version %d" % version)


def _create_signature_v1(secret, *parts):
    hash = hmac.new(utf8(secret), digestmod=hashlib.sha1)
    for part in parts:
        hash.update(utf8(part))
    return utf8(hash.hexdigest())


def _create_signature_v2(secret, s):
    hash = hmac.new(utf8(secret), digestmod=hashlib.sha256)
    hash.update(utf8(s))
    return utf8(hash.hexdigest())

```

version 1 和 version 2 都使用了 HMAC 进行签名，但是两者的加密方式一个为 sha1 一个为 sha256，都是不可逆的算法

下面假设 `time.time() = 1491747917` `cookie_secret = 'secret'` 的情况下调用 `self.set_secure_cookie('hello', 'world')` 对生成的 cookie 格式进行分析

#### version 1
cookie 的格式如下
`hello="d29ybGQ=|1491747917|ff266e2b3c35aaa9cd9e52d2347a6ec0e38ce76c";`  
可以按 `|` 将 value 拆分为 3 部分  
第一部分 `d29ybGQ` 为 `world` 进行 `base64` 后的结果  
第二部分 `1491747917` 即为当前时间的时间戳  
第三部分 `ff266e2b3c35aaa9cd9e52d2347a6ec0e38ce76c` 为 signature 的值，具体是由 `cookie_secret` 和 `value` 和 `timestamp` 进行签名后的结果  

#### version 2

cookie 的格式如下
`hello="2|1:0|10:1491747917|5:hello|8:d29ybGQ=|cd213a1d6e7604567841f10b80d558ea40cc715eb6dd1fa5040408c981d89e3f"`  
同样使用 `|` 拆分成为 6 部分  
第一部分 `2` 为版本号  
第二部分至第五部分均为 `len(value):value` 的形式，具体的 value 依次为 `key_version`, 时间戳, cookie 的键, cookie 的值(base64后)  
第六部分为 signature 的值  

从以上分析可以得出 Tornado 的 secure cookie 的内容是用户可见的，但是由于 signature 的存在我们无法在不知 `cookie_secret` 的情况下对 cookie 进行伪造(不过可以采取样本暴力猜解)。并且由于 version 2 中加入了 `key_version`， Tornado 可以支持多签名密钥，即使一个密钥被猜解，不会导致全部沦陷。

再来看看获取 cookie 的实现

```python
class RequestHandler(object):
    # ...
    def get_secure_cookie(self, name, value=None, max_age_days=31,
                          min_version=None):
        """Returns the given signed cookie if it validates, or None.

        The decoded cookie value is returned as a byte string (unlike
        `get_cookie`).

        .. versionchanged:: 3.2.1

           Added the ``min_version`` argument.  Introduced cookie version 2;
           both versions 1 and 2 are accepted by default.
        """
        self.require_setting("cookie_secret", "secure cookies")
        if value is None:
            value = self.get_cookie(name)
        return decode_signed_value(self.application.settings["cookie_secret"],
                                   name, value, max_age_days=max_age_days,
                                   min_version=min_version)

    def get_secure_cookie_key_version(self, name, value=None):
        """Returns the signing key version of the secure cookie.

        The version is returned as int.
        """
        self.require_setting("cookie_secret", "secure cookies")
        if value is None:
            value = self.get_cookie(name)
        return get_signature_key_version(value)



# A leading version number in decimal
# with no leading zeros, followed by a pipe.
_signed_value_version_re = re.compile(br"^([1-9][0-9]*)\|(.*)$")


def _get_version(value):
    # Figures out what version value is.  Version 1 did not include an
    # explicit version field and started with arbitrary base64 data,
    # which makes this tricky.
    m = _signed_value_version_re.match(value)
    if m is None:
        version = 1
    else:
        try:
            version = int(m.group(1))
            if version > 999:
                # Certain payloads from the version-less v1 format may
                # be parsed as valid integers.  Due to base64 padding
                # restrictions, this can only happen for numbers whose
                # length is a multiple of 4, so we can treat all
                # numbers up to 999 as versions, and for the rest we
                # fall back to v1 format.
                version = 1
        except ValueError:
            version = 1
    return version


def decode_signed_value(secret, name, value, max_age_days=31,
                        clock=None, min_version=None):
    if clock is None:
        clock = time.time
    if min_version is None:
        min_version = DEFAULT_SIGNED_VALUE_MIN_VERSION # 1
    if min_version > 2:
        raise ValueError("Unsupported min_version %d" % min_version)
    if not value:
        return None

    value = utf8(value)
    version = _get_version(value)

    if version < min_version:
        return None
    if version == 1:
        return _decode_signed_value_v1(secret, name, value,
                                       max_age_days, clock)
    elif version == 2:
        return _decode_signed_value_v2(secret, name, value,
                                       max_age_days, clock)
    else:
        return None


def _decode_signed_value_v1(secret, name, value, max_age_days, clock):
    parts = utf8(value).split(b"|") # base64_value, timestamp, signature
    if len(parts) != 3:
        return None
    signature = _create_signature_v1(secret, name, parts[0], parts[1])
    # python 3.3+ 中 _time_independent_equals 为 hmac.compare_digest 的一个 alias
    # other version 中 _time_independent_equals 是通过异或的方式比较是否相同
    if not _time_independent_equals(parts[2], signature):
        gen_log.warning("Invalid cookie signature %r", value)
        return None
    timestamp = int(parts[1])
    if timestamp < clock() - max_age_days * 86400:
        gen_log.warning("Expired cookie %r", value)
        return None
    # 下面两个分支感觉有点奇怪
    if timestamp > clock() + 31 * 86400:
        # _cookie_signature does not hash a delimiter between the
        # parts of the cookie, so an attacker could transfer trailing
        # digits from the payload to the timestamp without altering the
        # signature.  For backwards compatibility, sanity-check timestamp
        # here instead of modifying _cookie_signature.
        gen_log.warning("Cookie timestamp in future; possible tampering %r",
                        value)
        return None
    if parts[1].startswith(b"0"):
        gen_log.warning("Tampered cookie %r", value)
        return None
    try:
        return base64.b64decode(parts[0])
    except Exception:
        return None


def _decode_fields_v2(value):
    def _consume_field(s):
        # 按 : 分割成三部分(只分一次)
        # Example
        # In [40]: string = '1:0|10:1491747917'
        # In [41]: string.partition(':')
        # Out[41]: ('1', ':', '0|10:1491747917')
        length, _, rest = s.partition(b':')
        n = int(length)
        field_value = rest[:n]
        # In python 3, indexing bytes returns small integers; we must
        # use a slice to get a byte string as in python 2.
        if rest[n:n + 1] != b'|':
            raise ValueError("malformed v2 signed value field")
        rest = rest[n + 1:]
        return field_value, rest

    rest = value[2:]  # remove version number
    key_version, rest = _consume_field(rest)
    timestamp, rest = _consume_field(rest)
    name_field, rest = _consume_field(rest)
    value_field, passed_sig = _consume_field(rest)
    return int(key_version), timestamp, name_field, value_field, passed_sig


def _decode_signed_value_v2(secret, name, value, max_age_days, clock):
    try:
        key_version, timestamp, name_field, value_field, passed_sig = _decode_fields_v2(value)
    except ValueError:
        return None
    signed_string = value[:-len(passed_sig)]

    if isinstance(secret, dict):
        try:
            secret = secret[key_version]
        except KeyError:
            return None

    expected_sig = _create_signature_v2(secret, signed_string)
    if not _time_independent_equals(passed_sig, expected_sig):
        return None
    if name_field != utf8(name):
        return None
    timestamp = int(timestamp)
    if timestamp < clock() - max_age_days * 86400:
        # The signature has expired.
        return None
    try:
        return base64.b64decode(value_field)
    except Exception:
        return None


def get_signature_key_version(value):
    value = utf8(value)
    version = _get_version(value)
    if version < 2:
        return None
    try:
        key_version, _, _, _, _ = _decode_fields_v2(value)
    except ValueError:
        return None

    return key_version
```

`get_secure_cookie` 通过正向再生成 signature 并比对 cookie 中的 signature 来进行签名验证，并判断 cookie 是否还具有时效性。
再来看一个问题，我们看到在 `set_secure_cookie` 和 `get_secure_cookie` 中都有参数 `expires_days` 出现。为何这两个方法都需要这个参数，或者说仅仅依赖 `set_secure_cookie` 中的 `expires_days` 是否可行？

结果是不行的，虽然我们可以通过 `set_secure_cookie` 告诉 client 这个 cookie 的过期时间(client 会去遵守这个约定)。但这只是正常的情况，对于一个 hacker 来说并没有用，所以还需要在 `get_secure_cookie` 中再判断 cookie 是否已经过期。由于比对需要 cookie 生成的时间戳，所以两个 version 的 secure cookie 中都包含了时间戳部分。不过因为有 signature , hacker 不能强行更改时间戳

signature 不能被伪造全靠 `cookie_secret`，所以为了使 Tornado 的 secure cookie 更加安全，应当
1. 使用 version 2
2. 采用多签名密钥(即 `key_version` 和 `dict` 形式的 `cookie_secret`)
3. 增强 `cookie_secret` 的强度(防暴力猜解)
4. `cookie_secret` 的管理(防止密钥泄露)

~~感觉回到了搞渗透的2B岁月~~

    