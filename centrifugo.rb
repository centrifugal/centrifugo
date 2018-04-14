class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.9/centrifugo-1.7.9-darwin-amd64.zip"
  sha256 "584b3515385c7e38e1b0ee1672867503fda5646da5b8533085caef1fdfd0a273"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
