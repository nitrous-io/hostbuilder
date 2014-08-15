#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"
require "securerandom"

client = HTTPClient.new
client.receive_timeout = 10000

params = {
  id: "dfa7059d-1d11-479c-bd64-b4b62482e58f",
  name: "test-box-59c6928dbcd854c6",
}

resp = client.post "http://localhost:6666/hosts/restart", {
  provider: "virtualbox",
  config: JSON.generate(params)
}
p resp
