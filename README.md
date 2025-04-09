# Curing 💊
Curing is a POC of a rootkit that uses `io_uring` to perform different tasks without using any syscalls, making it invisible to security tools which are only monitoring syscalls.
The project was found effective against many of the most popular security tools such as Linux EDRs solutions and container security tools.
The idea was born at the latest CCC conference #38c3, therefor the name `Curing` which is a mix of `C` and `io_uring`.

To read the full article, check the [blog post](https://<todo>).

## POC
You can find a full demo of bypassing Falco with `curing` [here](poc/POC.md).
In the POC, you will also find the commands to build and run the `curing` client and server.

## Proving 0 syscalls
To prove that the rootkit is not using any syscalls, you can use the following command:
```bash
strace -f -o /tmp/strace.log ./build/client
```
0 syscalls is of course not possible, but the idea is to prove that the rootkit is not using any syscalls that are related to the attack, only the `io_uring` syscalls are used.

## How it works
The `curing` client is connecting to the `curing` server and is pulling commands from the server to execute. The server is sending commands to the client to read files, write files, create symbolic links, etc. The client is using `io_uring` to execute the commands and send the results back to the server.
Because the client is using `io_uring`, it is not using any syscalls that are related to the attack, making it invisible to security tools that are monitoring syscalls.
To know more about `io_uring`, you can check the [official documentation](https://kernel.dk/io_uring.pdf).

## Features
- [x] Read files
- [x] Write files
- [x] Create symbolic links
- [x] C2 server communication
- [ ] Execute processes ([blocked](https://github.com/axboe/liburing/discussions/1307))
- [ ] Any other feature from [here](https://github.com/axboe/liburing/blob/1a780b1fa6009fe9eb14dc48a99f6917556a8f3b/src/include/liburing/io_uring.h#L206)

## io_uring quick start
If you just want to play around with `io_uring` and test the security tool near to your house, you can use the example [here](io_uring_example/README.md).

## Requirements
- Linux kernel 5.1 or later

## Disclaimer
This project is a POC and should not be used for malicious purposes. The project is created to show how `io_uring` can be used to bypass security tools which are relying on syscalls.
We are not responsible for any kind of abuse of this project.