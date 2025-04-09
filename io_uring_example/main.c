#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>
#include "liburing.h"

#define QUEUE_DEPTH 1
#define BLOCK_SZ    1024

// Basic PDF magic bytes to make the file identifiable as PDF
const char pdf_magic[] = "%PDF-1.7\n"
                        "1 0 obj\n"
                        "<</Type /Catalog /Pages 2 0 R>>\n"
                        "endobj\n"
                        "2 0 obj\n"
                        "<</Type /Pages /Kids [] /Count 0>>\n"
                        "endobj\n"
                        "%%EOF\n";

int main() {
    struct io_uring ring;
    struct io_uring_sqe *sqe;
    struct io_uring_cqe *cqe;
    int ret, fd;

    // Initialize io_uring
    ret = io_uring_queue_init(QUEUE_DEPTH, &ring, 0);
    if (ret < 0) {
        fprintf(stderr, "queue_init: %s\n", strerror(-ret));
        return 1;
    }

    // Get an SQE for opening the file
    sqe = io_uring_get_sqe(&ring);
    if (!sqe) {
        fprintf(stderr, "Could not get SQE for open.\n");
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Prepare open operation
    io_uring_prep_openat(sqe, AT_FDCWD, "/tmp/shadow.pdf", 
                        O_WRONLY | O_CREAT | O_TRUNC, 0644);

    // Submit the open request
    ret = io_uring_submit(&ring);
    if (ret < 0) {
        fprintf(stderr, "submit open: %s\n", strerror(-ret));
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Wait for open completion
    ret = io_uring_wait_cqe(&ring, &cqe);
    if (ret < 0) {
        fprintf(stderr, "wait_cqe open: %s\n", strerror(-ret));
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Check if open was successful
    if (cqe->res < 0) {
        fprintf(stderr, "open: %s\n", strerror(-cqe->res));
        io_uring_cqe_seen(&ring, cqe);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Store the file descriptor
    fd = cqe->res;
    io_uring_cqe_seen(&ring, cqe);

    // Get an SQE for writing
    sqe = io_uring_get_sqe(&ring);
    if (!sqe) {
        fprintf(stderr, "Could not get SQE for write.\n");
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Prepare write operation
    io_uring_prep_write(sqe, fd, pdf_magic, strlen(pdf_magic), 0);

    // Submit the write request
    ret = io_uring_submit(&ring);
    if (ret < 0) {
        fprintf(stderr, "submit write: %s\n", strerror(-ret));
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Wait for write completion
    ret = io_uring_wait_cqe(&ring, &cqe);
    if (ret < 0) {
        fprintf(stderr, "wait_cqe write: %s\n", strerror(-ret));
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Check if write was successful
    if (cqe->res < 0) {
        fprintf(stderr, "write: %s\n", strerror(-cqe->res));
        io_uring_cqe_seen(&ring, cqe);
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    printf("Successfully wrote %d bytes to /tmp/shadow.pdf\n", cqe->res);

    // Mark completion as seen
    io_uring_cqe_seen(&ring, cqe);

    // Get an SQE for close
    sqe = io_uring_get_sqe(&ring);
    if (!sqe) {
        fprintf(stderr, "Could not get SQE for close.\n");
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Prepare close operation
    io_uring_prep_close(sqe, fd);

    // Submit the close request
    ret = io_uring_submit(&ring);
    if (ret < 0) {
        fprintf(stderr, "submit close: %s\n", strerror(-ret));
        close(fd);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Wait for close completion
    ret = io_uring_wait_cqe(&ring, &cqe);
    if (ret < 0) {
        fprintf(stderr, "wait_cqe close: %s\n", strerror(-ret));
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Check if close was successful
    if (cqe->res < 0) {
        fprintf(stderr, "close: %s\n", strerror(-cqe->res));
        io_uring_cqe_seen(&ring, cqe);
        io_uring_queue_exit(&ring);
        return 1;
    }

    // Mark completion as seen
    io_uring_cqe_seen(&ring, cqe);

    // Cleanup io_uring
    io_uring_queue_exit(&ring);

    return 0;
}
