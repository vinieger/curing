# Basic POC example of using io_uring

## Compile & Run
On a Linux system with kernel version 5.1 or later, you can compile and run the program with the following commands:
```
git clone https://github.com/axboe/liburing.git
make -C ./liburing
gcc main.c -o ./program -I./liburing/src/include/ -L./liburing/src/ -Wall -O2 -D_GNU_SOURCE -luring
./program
```

## Regular file creation
To see the flagging of the file creation, you can create a the file with the following command:
```
python3 -c 'open("/tmp/shadow.pdf","w").write("%PDF-1.7\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type /Pages /Kids [] /Count 0>>\nendobj\n%%EOF\n")'
```

## Bpftrace rule to monitor on open syscall
To monitor the open syscall, you can use the following bpftrace rule:
```
git clone git@github.com:bpftrace/bpftrace.git
sudo bpftrace -e 'tracepoint:syscalls:sys_enter_openat /str(args->filename) == "/tmp/shadow.pdf"/ { printf("Process: %s opened %s\n", comm, str(args->filename)); }'
```
You will see that the program that utilizes io_uring to open the file will not be captured by the bpftrace rule.
To test your security tool, you should define a rule to monitor this file creation, or change the code to open a sensitive file like `/etc/shadow`.
