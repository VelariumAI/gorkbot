#!/usr/bin/env python3
"""
Example Weather Tool for Gorkbot

This demonstrates how to write Python plugins for Gorkbot.
Uses the gorkbot_bridge framework.
"""

import sys
import os

# Add parent directory to path for gorkbot_bridge import
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run


# Example 1: Simple tool with basic parameters
@tool(description="Get current weather for a city")
def get_weather(city: str, units: str = "celsius") -> ToolResult:
    """
    Fetch weather data for the specified city.

    Args:
        city: Name of the city
        units: Temperature units (celsius/fahrenheit)

    Returns:
        ToolResult with weather information
    """
    # Simulated weather data (in real implementation, call weather API)
    weather_conditions = {
        "san francisco": {"temp": 18, "condition": "Partly Cloudy", "humidity": 65},
        "new york": {"temp": 22, "condition": "Sunny", "humidity": 45},
        "london": {"temp": 12, "condition": "Rainy", "humidity": 80},
        "tokyo": {"temp": 25, "condition": "Clear", "humidity": 55},
    }

    city_lower = city.lower()
    if city_lower in weather_conditions:
        data = weather_conditions[city_lower]

        # Convert temperature if needed
        temp = data["temp"]
        if units == "fahrenheit":
            temp = (temp * 9/5) + 32
            unit_symbol = "°F"
        else:
            unit_symbol = "°C"

        output = f"Weather in {city.title()}:\n"
        output += f"  Temperature: {temp}{unit_symbol}\n"
        output += f"  Condition: {data['condition']}\n"
        output += f"  Humidity: {data['humidity']}%"

        return ToolResult(
            success=True,
            output=output,
            data={
                "city": city,
                "temperature": temp,
                "units": units,
                "condition": data["condition"],
                "humidity": data["humidity"]
            }
        )
    else:
        return ToolResult(
            success=False,
            error=f"City not found in database: {city}"
        )


# Example 2: Tool with complex return data
@tool(description="Get 5-day weather forecast")
def get_forecast(city: str, days: int = 5) -> ToolResult:
    """
    Get a multi-day weather forecast.

    Args:
        city: City name
        days: Number of days (1-5)

    Returns:
        ToolResult with forecast data
    """
    if days < 1 or days > 5:
        return ToolResult(
            success=False,
            error="Days must be between 1 and 5"
        )

    # Simulated forecast data
    conditions = ["Sunny", "Cloudy", "Rainy", "Partly Cloudy", "Clear"]
    base_temp = 20

    forecast = []
    for i in range(days):
        forecast.append({
            "day": i + 1,
            "high": base_temp + (i % 3) * 2,
            "low": base_temp - 2 - (i % 2),
            "condition": conditions[i % len(conditions)]
        })

    output = f"5-Day Forecast for {city}:\n"
    for day in forecast:
        output += f"  Day {day['day']}: {day['condition']}, "
        output += f"High: {day['high']}°C, Low: {day['low']}°C\n"

    return ToolResult(
        success=True,
        output=output,
        data={"city": city, "forecast": forecast}
    )


# Example 3: Tool that can fail gracefully
@tool(description="Check if it's a good day for outdoor activities")
def outdoor_check(city: str) -> ToolResult:
    """
    Analyze weather conditions and suggest if outdoor activities are recommended.
    """
    # Use our weather function logic
    weather_data = {
        "san francisco": {"temp": 18, "condition": "Partly Cloudy", "humidity": 65},
        "new york": {"temp": 22, "condition": "Sunny", "humidity": 45},
    }

    city_lower = city.lower()
    if city_lower not in weather_data:
        return ToolResult(
            success=False,
            error=f"Unknown city: {city}"
        )

    data = weather_data[city_lower]

    # Simple recommendation logic
    recommendations = []
    score = 0

    if 15 <= data["temp"] <= 28:
        score += 2
        recommendations.append("✓ Temperature is pleasant")
    else:
        score -= 1
        recommendations.append("✗ Temperature is extreme")

    if data["humidity"] < 70:
        score += 1
        recommendations.append("✓ Humidity is comfortable")
    else:
        recommendations.append("✗ High humidity")

    if "Rainy" not in data["condition"]:
        score += 2
        recommendations.append("✓ No rain expected")
    else:
        score -= 1
        recommendations.append("✗ Rain expected")

    if score >= 4:
        verdict = "EXCELLENT"
    elif score >= 2:
        verdict = "GOOD"
    elif score >= 0:
        verdict = "FAIR"
    else:
        verdict = "NOT RECOMMENDED"

    output = f"Outdoor Activity Assessment for {city.title()}:\n"
    output += f"  Verdict: {verdict}\n"
    output += f"  Score: {score}/7\n"
    output += "  Details:\n"
    for rec in recommendations:
        output += f"    {rec}\n"

    return ToolResult(
        success=True,
        output=output,
        data={"city": city, "verdict": verdict, "score": score}
    )


# Main entry point - handles communication with Go
if __name__ == "__main__":
    run()
