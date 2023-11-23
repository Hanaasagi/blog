
+++
title = "Unix Domain Socket"
summary = ''
description = ""
categories = []
tags = []
date = 2016-06-03T08:33:22+08:00
draft = false
+++

*Unix domain socket 或者 IPC socket是一种终端，可以使同一台操作系统上的两个或多个进程进行数据通信。与管道相比，Unix domain socket 既可以使用字节流，又可以使用数据队列，而管道通信则只能使用字节流。Unix domain socket的接口和Internet socket很像，但它不使用网络底层协议来通信。Unix domain socket 的功能是POSIX操作系统里的一种组件。  
Unix domain socket 使用系统文件的地址来作为自己的身份。它可以被系统进程引用。所以两个进程可以同时打开一个Unix domain sockets来进行通信。不过这种通信方式是发生在系统内核里而不会在网络里传播。【from wiki】*  

如果从编程的角度来看，Unix domain socket和TCP/IP之间有两个重要的不同。第一点，socket的地址是文件路径，而不是一个包含ip和端口的元组。其次， 在文件系统创建的代表socket的文件在socket关闭后依然存在，并且需要在每次服务器启动时删除。


server.py  

    import socket
    import sys
    import os
    
    SERVER_PATH = './uds_socket'
    
    # Make sure the socket does not already exist
    if os.path.exists(SERVER_PATH):
    	os.remove(SERVER_PATH)
    print >>sys.stderr ,'starting unix domain socket server'
    
    # Create a UDS socket
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # Bind the socket to the port
    sock.bind(SERVER_PATH)
    # Listen for incoming conns
    sock.listen(2)
    print >>sys.stderr, 'Listening on path: %s' % SERVER_PATH
    
    while True:
        conn, addr = sock.accept()
        try:
            while True:
                data = conn.recv(1024)
                if data:
                    print >>sys.stderr, 'received [%s]' % data
                    conn.sendall(data)
                else:
                    break
        except Exception,e:
         	 print e
        finally:
            conn.close()
            
            
            
client.py  

    import socket
    import sys
    
    # Create a UDS socket
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # Connect the socket to the port where the server is listening
    server_address = './uds_socket'
    print >>sys.stderr, 'connecting to %s' % server_address
    try:
        sock.connect(server_address)
    except socket.error, msg:
        print >>sys.stderr, msg
        sys.exit(1)
        
    try:
        # Send data
        message = 'This is the message, This will be echoed back'
        print >>sys.stderr, 'sending [%s]' % message
        sock.sendall(message)
    
        amount_received = 0
        amount_expected = len(message)
        
        while amount_received < amount_expected:
            data = sock.recv(1024)
            amount_received += len(data)
            print >>sys.stderr, 'received [%s]' % data
    
    finally:
        print >>sys.stderr, 'closing socket'
        sock.close()
    