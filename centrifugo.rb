class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.6.1/centrifugo-1.6.1-darwin-amd64.zip"
  sha256 "e2c9467d996bbb62416544d1e6a53e6044c90e405a4f62a3ed651485526f5395"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
