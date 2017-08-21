class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.4/centrifugo-1.7.4-darwin-amd64.zip"
  sha256 "ba25547f33568888fd47e7e7ec9747a4d57bc7749cb9e2cc380535228359b41c"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
