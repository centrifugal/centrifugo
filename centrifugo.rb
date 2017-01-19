class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.6.3/centrifugo-1.6.3-darwin-amd64.zip"
  sha256 "98bf03e36a5b55ecc98e6314725b1bc77d1b224823827a9b7b4bbc57571422d5"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
