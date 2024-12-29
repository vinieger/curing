# curing
Zero syscalls malware utilizing `io_uring` for the task.

## Description
`curing` is a simple POC of a malware that uses `io_uring` to perform different tasks without using any syscalls. The malware is able to read files, write files, and execute commands without using any syscalls. The malware is able to perform these tasks by using the `io_uring` interface to perform the necessary I/O operations. In addition to this, curing is able to connect back to a C2 server and receive commands to execute.
