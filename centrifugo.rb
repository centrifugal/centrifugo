class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.6.4/centrifugo-1.6.4-darwin-amd64.zip"
  sha256 "c5a13e2d4e2fd7513909c3bd4fec545d4996dae7cf035efb4d71bea6df38cfcc"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
