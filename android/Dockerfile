# Toolchain image for building the Natlas APK: JDK 17 + Android SDK 34 + Go.
# It holds no source code; build.sh mounts the repo into it.
FROM eclipse-temurin:17-jdk-jammy

ARG GO_VERSION=1.24.7
ARG CMDLINE_TOOLS=11076708

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    PATH=/usr/local/go/bin:/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/build-tools/34.0.0:$PATH

# Basics.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl unzip git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Go (builds the engine for each phone CPU).
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz

# Android command-line tools, platform 34 and build-tools 34.0.0 (licences accepted).
RUN mkdir -p "$ANDROID_HOME/cmdline-tools" \
 && curl -fsSLo /tmp/tools.zip "https://dl.google.com/android/repository/commandlinetools-linux-${CMDLINE_TOOLS}_latest.zip" \
 && unzip -q /tmp/tools.zip -d "$ANDROID_HOME/cmdline-tools" \
 && mv "$ANDROID_HOME/cmdline-tools/cmdline-tools" "$ANDROID_HOME/cmdline-tools/latest" \
 && rm /tmp/tools.zip \
 && (yes | sdkmanager --licenses > /dev/null) \
 && sdkmanager --install "platforms;android-34" "build-tools;34.0.0" "platform-tools" > /dev/null \
 && chmod -R a+rX "$ANDROID_HOME"

WORKDIR /src
