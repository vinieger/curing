# curing
Zero syscalls malware utilizing `io_uring`.

## Description
`curing` is a simple POC of a malware that uses `io_uring` to perform different tasks without using any syscalls. The malware is able to read files, write files, and execute commands without using any syscalls. The malware is able to perform these tasks by using the `io_uring` interface to perform the necessary I/O operations. In addition to this, curing is able to connect back to a C2 server and receive commands to execute.

## Usage
To compile the malware, you can use the makefile provided. The makefile will compile the malware and the C2 server. To compile the malware, you can use the following command:
```bash
make all
```
You will see the binaries in the `build` directory (`client` and `server`).

To run the C2 server, you can use the following command:
```bash
./build/server <port>
```

To run the malware, you can use the following command:
```bash
./build/client
```
The configuration of the client is in the `cmd/config.json` file. You can change the configuration to connect to the C2 server and set the interval to connect to the C2 server.

## Proving 0 syscalls
To prove that the malware is not using any syscalls, you can use the following command:
```bash
strace -f -o /tmp/strace.log ./build/client
```

## Features
- [x] Read files
- [x] Write files
- [x] Create symbolic links
- [x] C2 server communication
- [ ] Execute commands ([blocked](https://github.com/axboe/liburing/discussions/1307))