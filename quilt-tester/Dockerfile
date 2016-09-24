From quilt/quilt
Maintainer Ethan J. Jackson

RUN VER=1.6.2 \
&& export GOROOT=/tmp/build/go GOPATH=/tmp/build/gowork \
&& export PATH=$PATH:$GOROOT/bin \
&& apk update \
&& apk add --no-cache bash git python curl openssh-client \
# Alpine uses musl instead of glibc which confuses go.
# They're compatible, so just symlink
&& mkdir /tmp/build && cd /tmp/build \
&& wget https://storage.googleapis.com/golang/go$VER.linux-amd64.tar.gz \
&& gunzip -c go$VER.linux-amd64.tar.gz | tar x \
&& go get -u github.com/NetSys/quilt \
&& cd $GOPATH/src/github.com/NetSys/quilt/quilt-tester \
&& go build -o /bin/quilt-tester . \
&& mkdir /tests \
&& for suite in tests/* ; do \
	for f in $suite/* ; do \
		extension=$(echo "$f" | awk -F'.' '{print $NF}') ; \
		if [ $extension == "go" ] ; then \
			go build -v -o ${f%???} $f ; \
		fi; \
	done; \
	cp -r $suite /tests ; \
done \
&& rm -rf /tmp/build
Copy config/id_rsa /root/.ssh/id_rsa
Copy "~/.slack_hook" "~/.slack_hook"
RUN chmod 0600 /root/.ssh/id_rsa

Entrypoint ["quilt-tester"]
