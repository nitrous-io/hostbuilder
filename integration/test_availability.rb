#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new

resp = client.get "http://localhost:6666/availability"
p JSON.parse(resp.body)
