class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.0/centrifugo-1.7.0-darwin-amd64.zip"
  sha256 "50025bfbd93171ed49d9c7b2c3a65b271b590414db09268bfa60ee0f14fcc6aa"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
