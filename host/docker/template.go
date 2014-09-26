package docker

import "text/template"

const dockerfileTmpl = `
FROM nitrousio/ubuntu-dind:14.04

RUN wget http://nitrous-public.s3.amazonaws.com/docker-6df533a -O /usr/bin/docker

ENV HOME /root

RUN mkdir -p /etc/ssh
RUN touch /etc/ssh/univ_public_keys
{{ range .PublicKeys }}
RUN echo -e "{{.}}" >> /etc/ssh/univ_public_keys
{{ end }}

RUN mkdir -p /etc/freighterd

{{ if .TLS }}
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.CA}} > /etc/freighterd/ca.crt'
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.Cert}} > /etc/freighterd/cert.crt'
RUN /bin/bash -c 'echo -e {{printf "%q" .TLS.Key}} > /etc/freighterd/key.crt'
{{ end }}

{{ if .Secret }}
RUN /bin/bash -c 'echo -e {{printf "%q" .Secret}} > /etc/freighterd/secret'
{{ end }}

RUN /bin/bash -c "echo -e '#!/bin/bash\nrm /var/run/docker.pid\ndocker -d >& /var/log/docker.log &\n/freighterd' > /start-freighterd" && chmod +x /start-freighterd

EXPOSE 65333
`

var dockerfileTemplate *template.Template

func init() {
	dockerfileTemplate =
		template.Must(template.New("dockerfile").Parse(dockerfileTmpl))
}
