package docker

import "text/template"

const dockerfileTmpl = `
FROM nitrousio/ubuntu-dind:14.04

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

RUN /bin/bash -c "echo -e '#!/bin/bash\nrm /var/run/docker.pid\ndocker -d -e lxc >& /var/log/docker.log &\n/freighterd' > /start-freighterd" && chmod +x /start-freighterd

EXPOSE 65333
`

var dockerfileTemplate *template.Template

func init() {
	dockerfileTemplate =
		template.Must(template.New("dockerfile").Parse(dockerfileTmpl))
}
