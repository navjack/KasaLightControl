# Kasa Light Control Application

## Project Goal
Develop a GUI-based application that fully replicates the functionality of an existing PowerShell script for controlling TPLink Kasa smart bulbs locally, eliminating the dependency on the Python-Kasa framework.

## Project Description
This project aims to understand the protocol and communication methods employed by TPLink Kasa devices (as potentially used by libraries like Python-Kasa) and to independently implement this functionality directly in GoLang. The initial development will focus on GoLang for its cross-platform capabilities, with potential future ports or expansions into Swift for native macOS GUI integration.

## Key Features to Implement:

*   Reverse-engineer and document the network protocols used by Kasa devices.
*   Full local control capabilities including:
    *   Turning devices on/off.
    *   Adjusting brightness, color temperature, and color settings.
    *   Retrieving device state and power usage (where applicable).
*   Device discovery on the local network.
*   Historical energy usage monitoring (for applicable devices).
*   A clean, user-friendly GUI.
