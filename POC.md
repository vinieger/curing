# POC bypassing Falco

```shell
docker run --rm -i -t --name falco --privileged  \
    -v /var/run/docker.sock:/host/var/run/docker.sock \
    -v /dev:/host/dev -v /proc:/host/proc:ro -v /boot:/host/boot:ro \
    -v /lib/modules:/host/lib/modules:ro -v /usr:/host/usr:ro -v /etc:/host/etc:ro \
    falcosecurity/falco-no-driver:0.39.2 falco
```

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
