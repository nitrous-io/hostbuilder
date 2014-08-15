#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"
require "securerandom"

client = HTTPClient.new
client.receive_timeout = 10000

random_id = SecureRandom.hex(8)

params = {
  name: "test-box-#{random_id}",
  cpus: 2,
  memory: 512
}

resp = client.post "http://localhost:6666/hosts", {
  provider: "virtualbox",
  config: JSON.generate(params)
}
p resp
