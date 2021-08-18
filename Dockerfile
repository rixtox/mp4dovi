FROM scratch
COPY mp4dovi /usr/bin/mp4dovi
ENTRYPOINT ["/usr/bin/mp4dovi"]
